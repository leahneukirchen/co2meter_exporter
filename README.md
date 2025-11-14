# co2meter_exporter
CO2 Meter state exporter to use with https://prometheus.io/

Keep track of CO2 level around you to stay productive!

![Picture of cheap co2meter](https://user-images.githubusercontent.com/22738239/73683926-7a247c80-46c3-11ea-99cb-a086262aa693.jpg)

## CO2 meters supported
All devices that report as USB-zyTemp should in theory be supported.

```
Bus 001 Device 004: ID 04d9:a052 Holtek Semiconductor, Inc. USB-zyTemp
```

I've played with [this](https://www.wetterladen.de/aircontrol-mini-co2-messgeraet-tfa-31.5006-plus-incl-stecker-netzteil-raumklimakontrolle) one.
All of them look more or less same and don't cost too much. Some of them will also report humidity, most will not.

It is best to power this device via Raspberry Pi in-the-middle, so no extra power supply is needed.

## Running in a docker container

Raspberry Pi and docker are great friends. Just run docker container and you will have Prometheus exporter
at no cost!

```
docker run -dt -p 2112:2112/tcp --name co2meter_exporter --restart unless-stopped --privileged imple/co2meter_exporter:latest
```

## Running and serving metrics

```
% ./co2monitor --help
Usage of ./co2meter_exporter:
  -d string
    	device to get readings from
  -h string
    	host to bind to (default "::")
  -p string
    	port to bind to (default "9200")
  -q	quiet mode (no periodic output)
  -skip-decryption
    	skip value decryption. This is needed for some CO2 meter models.

% ./co2monitor -d /dev/hidraw0 -p 2112
2020/02/03 19:07:46 Listening on http://0.0.0.0:2112/metrics
2020/02/03 19:07:51 CO2 reading:  527
2020/02/03 19:07:51 Temperature reading:  19.48
2020/02/03 19:07:56 CO2 reading:  527
2020/02/03 19:07:56 Temperature reading:  19.48
2020/02/03 19:08:01 CO2 reading:  527
2020/02/03 19:08:01 Temperature reading:  19.48
2020/02/03 19:08:06 CO2 reading:  527
2020/02/03 19:08:06 Temperature reading:  19.48
2020/02/03 19:08:11 CO2 reading:  529
```

Get [Prometheus](https://prometheus.io/), [Grafana](https://grafana.com/), and finish setup!

![Screenshot](https://user-images.githubusercontent.com/22738239/73684030-aa6c1b00-46c3-11ea-9d7d-e4a4cdd87fa7.png)

