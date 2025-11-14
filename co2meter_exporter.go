package main

// Following code is based on this great work:
// https://hackaday.io/project/5301-reverse-engineering-a-low-cost-usb-co-monitor/log/17909-all-your-base-are-belong-to-us

import (
	"crypto/rand"
	"encoding/binary"
	"flag"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	readingInterval = time.Millisecond * 200
	reportInterval  = time.Second * 5
)

type envState struct {
	sync.RWMutex
	co2         int
	temperature float64
}

func (s *envState) Co2() int {
	s.RLock()
	defer s.RUnlock()
	return s.co2
}

func (s *envState) setCo2(co2 int) {
	s.Lock()
	defer s.Unlock()
	s.co2 = co2
}

func (s *envState) Temperature() float64 {
	s.RLock()
	defer s.RUnlock()
	return s.temperature
}

func (s *envState) setTemperature(temperature float64) {
	s.Lock()
	defer s.Unlock()
	s.temperature = temperature
}

func decryptReading(buffer []byte, key []byte) []byte {
	var cstate = []byte{0x48, 0x74, 0x65, 0x6D, 0x70, 0x39, 0x39, 0x65}
	var shuffle = []byte{2, 4, 0, 7, 1, 6, 5, 3}

	phase1 := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	for i, j := range shuffle {
		phase1[j] = buffer[i]
	}

	phase2 := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	for i := range shuffle {
		phase2[i] = phase1[i] ^ key[i]
	}

	phase3 := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	for i := range shuffle {
		phase3[i] = ((phase2[i] >> 3) | (phase2[(i-1+8)%8] << 5)) & 0xff
	}

	ctmp := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	for i := range shuffle {
		ctmp[i] = ((cstate[i] >> 4) | (cstate[i] << 4)) & 0xff
	}

	out := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	for i := range shuffle {
		out[i] = (byte)(((0x100 + (int)(phase3[i]) - (int)(ctmp[i])) & (int)(0xff)))
	}

	return out
}

func isValidReading(buffer []byte) bool {
	if buffer[4] != 0x0D || (buffer[0]+buffer[1]+buffer[2])&0xFF != buffer[3] {
		return false
	}

	return true
}

func hidSetReport(source *os.File, key []byte) {
	// Prepare report buffer. Buffer cannot be slice object, since it will be
	// passed to kernel

	var report [9]byte    // we will send this report to ioctl HIDIOCSFEATURE(9)
	report[0] = 0x00      // report number shall always be zero
	copy(report[1:], key) // rest of report is random 8 byte key

	// Issue HID SET_REPORT on device
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(source.Fd()),
		// Following ioctl call number is equivalent to HIDIOCSFEATURE(9)
		// more info: https://www.kernel.org/doc/Documentation/hid/hidraw.txt
		uintptr(0xC0094806),
		uintptr(unsafe.Pointer(&report)),
	)
	if errno != 0 {
		log.Fatal("ioctl failed: ", errno)
	}
}

func getReadings(source *os.File, key []byte, s *envState, skipDecryption bool) {
	buffer := make([]byte, 8)

	for {
		// Every data measurement from device comes in 8 byte chunks
		_, err := io.ReadFull(source, buffer)
		if err != nil {
			log.Fatal(err)
		}

		var code byte
		var value int
		if skipDecryption {
			code = buffer[0]
			value = int(binary.BigEndian.Uint16(buffer[1:3]))
		} else {
			decrypted := decryptReading(buffer, key)

			if !isValidReading(decrypted) {
				log.Println("Data decryption failed: ", decrypted)
				break
			}

			code = decrypted[0]
			value = int(binary.BigEndian.Uint16(decrypted[1:3]))
		}

		switch code {
		case 0x50:
			// Got CO2 reading (code 0x50)
			s.setCo2(value)
		case 0x42:
			// Got temperature reading (code 0x42)
			s.setTemperature(math.Round((float64(value)/16.0-273.15)*100) / 100)
		}
		time.Sleep(readingInterval)
	}
}

func logMetrics(s *envState) {
	co2Gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "co2meter_co2_ppms",
		Help: "CO2 reading in PPM.",
	})

	temperatureGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "co2meter_temperature_celsius",
		Help: "Temperature reading in degree celsius.",
	})

	prometheus.MustRegister(temperatureGauge)
	prometheus.MustRegister(co2Gauge)

	for {
		time.Sleep(reportInterval)

		co2 := s.Co2()
		t := s.Temperature()

		log.Printf("CO2: %d ppm,\tTemperature: %.02f C\n", co2, t)

		co2Gauge.Set(float64(co2))
		temperatureGauge.Set(t)
	}
}

var deviceFlag = flag.String("d", "", "device to get readings from")
var hostFlag = flag.String("h", "::", "host to bind to")
var portFlag = flag.String("p", "9200", "port to bind to")
var skipDecryptionFlag = flag.Bool("skip-decryption", false, "skip value decryption. This is needed for some CO2 meter models.")

func main() {
	var key [8]byte
	var state envState

	flag.Parse()

	if *deviceFlag == "" {
		log.Fatal("missing device path")
	}
	source, err := os.OpenFile(*deviceFlag, os.O_RDWR, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer source.Close()

	// Generate random key
	rand.Read(key[:])

	hidSetReport(source, key[:])

	go getReadings(source, key[:], &state, *skipDecryptionFlag)
	go logMetrics(&state)

	log.Printf("Listening on http://%s/metrics\n", net.JoinHostPort(*hostFlag, *portFlag))

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(net.JoinHostPort(*hostFlag, *portFlag), nil)
}
