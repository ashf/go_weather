package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/rubenv/opencagedata"
)

type weatherData struct {
	Name string `json:"name"`
	Main struct {
		Kelvin float64 `json:"temp"`
	} `json:"main"`
}

type weatherProvider interface {
	temperature(city string) (float64, error) // in Kelvin, naturally
}

type multiWeatherProvider []weatherProvider

type openWeatherMap struct {
	apiKey string
}
type weatherBitMap struct {
	apiKey string
}
type climaCellMap struct {
	apiKey string
}

func main() {
	http.HandleFunc("/hello", hello)
	http.HandleFunc("/weather/", handleWeather)

	http.ListenAndServe(":3000", nil)
}

func hello(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello!"))
}

func handleWeather(w http.ResponseWriter, r *http.Request) {
	mw := multiWeatherProvider{
		openWeatherMap{apiKey: "4dd289639d38eff5372a5e5e082e66a2"},
		weatherBitMap{apiKey: "15149aaf102f4822bd6175807899c0ea"},
		climaCellMap{apiKey: "ln72tf5m8L8KixwmfhmKE6A3S9Gs28g1"},
	}

	begin := time.Now()
	city := strings.SplitN(r.URL.Path, "/", 3)[2]

	temp, err := mw.temperature(city)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"city": city,
		"temp": temp,
		"took": time.Since(begin).String(),
	})
}

func (w multiWeatherProvider) temperature(city string) (float64, error) {
	// Make a channel for temperatures, and a channel for errors.
	// Each provider will push a value into only one.
	temps := make(chan float64, len(w))
	errs := make(chan error, len(w))

	// For each provider, spawn a goroutine with an anonymous function.
	// That function will invoke the temperature method, and forward the response.
	for _, provider := range w {
		go func(p weatherProvider) {
			k, err := p.temperature(city)
			if err != nil {
				errs <- err
				return
			}
			temps <- k
		}(provider)
	}

	sum := 0.0

	// Collect a temperature or an error from each provider.
	for i := 0; i < len(w); i++ {
		select {
		case temp := <-temps:
			sum += temp
		case err := <-errs:
			return 0, err
		}
	}

	// Return the average, same as before.
	return sum / float64(len(w)), nil
}

func (w openWeatherMap) temperature(city string) (float64, error) {
	resp, err := http.Get("http://api.openweathermap.org/data/2.5/weather?APPID=" + w.apiKey + "&q=" + city)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var d struct {
		Main struct {
			Kelvin float64 `json:"temp"`
		} `json:"main"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	log.Printf("openWeatherMap: %s: %.2f", city, d.Main.Kelvin)
	return d.Main.Kelvin, nil
}

func (w weatherBitMap) temperature(city string) (float64, error) {
	resp, err := http.Get("https://api.weatherbit.io/v2.0/current?units=S&city=" + city + "&key=" + w.apiKey)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var d struct {
		Data []struct {
			Kelvin float64 `json:"temp"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	log.Printf("weatherBitMap: %s: %.2f", city, d.Data[0].Kelvin)
	return d.Data[0].Kelvin, nil
}

func (w climaCellMap) temperature(city string) (float64, error) {
	apiKey := "cc41b2733acb4b388644a4eb94b7348e"
	geocoder := opencagedata.NewGeocoder(apiKey)

	result, err0 := geocoder.Geocode(city, nil)
	if err0 != nil {
		return 0, err0
	}

	lat := fmt.Sprintf("%v", result.Results[0].Geometry.Latitude)
	lon := fmt.Sprintf("%v", result.Results[0].Geometry.Longitude)

	resp, err := http.Get("https://api.climacell.co/v3/weather/realtime?lat=" + lat + "&lon=" + lon + "&fields=temp&apikey=" + w.apiKey)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	// bodyBytes, err2 := ioutil.ReadAll(resp.Body)
	// if err2 != nil {
	// 	log.Fatal(err2)
	// }
	// bodyString := string(bodyBytes)
	// log.Println(bodyString)

	var d struct {
		Temp struct {
			Celcius float64 `json:"value"`
		} `json:"temp"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		log.Println("error decoding")
		return 0, err
	}

	d.Temp.Celcius += 273.15

	log.Printf("climaCellMap: %s: %.2f", city, d.Temp.Celcius)
	return d.Temp.Celcius, nil
}
