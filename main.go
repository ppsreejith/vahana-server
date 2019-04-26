package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dhconnelly/rtreego"
	"github.com/gorilla/mux"
)

func main() {
	r := mux.NewRouter()
	stops := LoadStops("./resources/stops.json")
	rt := createLatLngTree()
	r.HandleFunc("/routes/{latlng}", GetRoutesHandler(stops, rt))
	initServer(r)
}

type Somewhere struct {
	location rtreego.Point
	name     string
	wormhole chan int
}

const tol = 0.01

func (s *Somewhere) Bounds() *rtreego.Rect {
	return s.location.ToRect(tol)
}

func createLatLngTree() *rtreego.Rtree {
	rt := rtreego.NewTree(2, 25, 50)
	rt.Insert(&Somewhere{rtreego.Point{0, 0}, "Someplace 0 0", nil})
	rt.Insert(&Somewhere{rtreego.Point{1, 1}, "Someplace 1 1", nil})
	rt.Insert(&Somewhere{rtreego.Point{1, 0}, "Someplace 1 0", nil})
	rt.Insert(&Somewhere{rtreego.Point{0, 1}, "Someplace 0 1", nil})
	return rt
}

func initServer(r *mux.Router) {
	srv := &http.Server{
		Addr: "0.0.0.0:8080",
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      r, // Pass our instance of gorilla/mux in.
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Println(err)
	}
}

func GetRoutesHandler(stops []Stop, rt *rtreego.Rtree) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		latlngStr := params["latlng"]
		if latlngStr == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		latlng := strings.Split(latlngStr, ",")
		latStr := latlng[0]
		lngStr := latlng[1]
		lat, err := strconv.ParseFloat(latStr, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		lng, err := strconv.ParseFloat(lngStr, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		data, err := json.Marshal(map[string]int{
			"totalsize": len(stops),
			"rtreesize": rt.Size(),
			"lat":       int(lat),
			"lng":       int(lng),
		})
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}
}

type Position struct {
	Longitude float64 `json:"Longitude"`
	Latitude  float64 `json:"Latitude"`
}

type Stop struct {
	Name     string   `json:"StopPointName"`
	Location Position `json:"Location"`
}

func LoadStops(file string) []Stop {
	var stops []Stop
	configFile, err := os.Open(file)
	defer configFile.Close()
	if err != nil {
		fmt.Println(err.Error())
	}
	jsonParser := json.NewDecoder(configFile)
	jsonParser.Decode(&stops)
	return stops
}
