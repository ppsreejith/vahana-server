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

type PointsMap = map[string]RTreePoint

func main() {
	r := mux.NewRouter()
	stops := LoadStops("./resources/stops.json")
	rt, pointsMap := createLatLngTree(stops)
	r.HandleFunc("/routes/{latlng}", GetRoutesHandler(stops, rt, pointsMap))
	initServer(r)
}

type RTreePoint struct {
	location rtreego.Point
	name     string
}

const tol = 2

func (s RTreePoint) Bounds() *rtreego.Rect {
	return s.location.ToRect(tol)
}

func createLatLngTree(stops []Stop) (*rtreego.Rtree, PointsMap) {
	rt := rtreego.NewTree(2, 25, 50)
	points := []RTreePoint{}
	for _, stop := range stops {
		points = append(points, RTreePoint{rtreego.Point{stop.Location.Latitude, stop.Location.Longitude}, stop.Name})
	}
	// RTreePoint{rtreego.Point{0, 0}, "Someplace 0 0"},
	// RTreePoint{rtreego.Point{1, 0}, "Someplace 1 0"},
	// RTreePoint{rtreego.Point{1, 1}, "Someplace 1 1"},
	// RTreePoint{rtreego.Point{0, 1}, "Someplace 0 1"},
	pointsMap := make(PointsMap)
	for _, point := range points {
		pointsMap[point.location.ToRect(tol).String()] = point
		rt.Insert(point)
	}
	return rt, pointsMap
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

func GetRoutesHandler(stops []Stop, rt *rtreego.Rtree, pointsMap PointsMap) http.HandlerFunc {
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
		results := rt.NearestNeighbors(3, rtreego.Point{lat, lng})
		var resultPoints []RTreePoint
		for _, result := range results {
			rect := result.Bounds()
			resultPoints = append(resultPoints, pointsMap[rect.String()])
		}
		data, err := json.Marshal(map[string]string{
			"closes point 1": resultPoints[0].name,
			"closes point 2": resultPoints[1].name,
			"closes point 3": resultPoints[2].name,
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
