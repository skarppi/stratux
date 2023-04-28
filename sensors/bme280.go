// Package sensors provides a stratux interface to sensors used for AHRS calculations.
package sensors

import (
	"errors"
	"time"

	"github.com/kidoman/embd"
	"tinygo.org/x/drivers/bme280"
)

// BME280 represents a BME280 sensor and implements the PressureSensor interface.
type BME280 struct {
	i2cbus *embd.I2CBus
	sensor  *bme280.Device
	stopFunc func()
	running bool
}

var errBME = errors.New("BME280 Error: BME280 is not running")

// NewBME280 looks for a BME280 connected on the I2C bus having one of the valid addresses and begins reading it.
func NewBME280(i2cbus *embd.I2CBus, freq time.Duration) (bme *BME280, err error) {

	bme = new(BME280)
	bme.i2cbus = i2cbus
	bme.running = true
	bme.stopFunc = func () {
		println("BMP280 disconnected")
		bme.sensor.Reset()
	}
	
	//machine.I2C0.Configure(drivers.I2CConfig{})
    sensor := bme280.New(bme)
    sensor.ConfigureWithSettings(bme280.Config{
		Mode:        bme280.ModeNormal,
		Period:      bme280.Period125ms,
		Temperature: bme280.Sampling2X,
		Humidity:    bme280.Sampling1X,
		Pressure:    bme280.Sampling16X,
		IIR:         bme280.Coeff16,
	})

    if !sensor.Connected() {
		sensor.Address = 0x77

		if !sensor.Connected() {
        	println("BMP280 not detected")
			err = errBME
        	return
		}
    }
    println("BMP280 detected")
	bme.sensor = &sensor
	
	return
}

// Temperature returns the current temperature in degrees C measured by the BME280
func (bme *BME280) Temperature() (float64, error) {
	if !bme.running {
		return 0, errBME
	}
	temp, _ := bme.sensor.ReadTemperature()
	return float64(temp)/1000, nil
}

// Pressure returns the current pressure in mbar measured by the BME280
func (bme *BME280) Pressure() (float64, error) {
	if !bme.running {
		return 0, errBME
	}

	pressure, _ := bme.sensor.ReadPressure()
	return float64(pressure)/100000, nil
}

// Close stops the measurements of the BME280
func (bme *BME280) Close() {
	bme.running = false
	bme.stopFunc()
}

func (bme *BME280) ReadRegister(addr uint8, r uint8, buf []byte) error {
	errRead := (*bme.i2cbus).ReadFromReg(byte(addr), r, buf)
	return errRead
}

func (bme *BME280) WriteRegister(addr uint8, r uint8, buf []byte) error {
	return (*bme.i2cbus).WriteToReg(byte(addr), r, buf)
}

func (bme *BME280) Tx(addr uint16, w, r []byte) error {
	return nil
}