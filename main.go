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
	r.HandleFunc("/routes/{latlng1}/{latlng2}", GetRoutesHandler(stops, rt, pointsMap))
	initServer(r)
}

type RTreePoint struct {
	location rtreego.Point
	stop     Stop
}

const tol = 2

func (s RTreePoint) Bounds() *rtreego.Rect {
	return s.location.ToRect(tol)
}

func createLatLngTree(stops []Stop) (*rtreego.Rtree, PointsMap) {
	rt := rtreego.NewTree(2, 25, 50)
	points := []RTreePoint{}
	for _, stop := range stops {
		points = append(points, RTreePoint{rtreego.Point{stop.Location.Latitude, stop.Location.Longitude}, stop})
	}
	pointsMap := make(PointsMap)
	for _, point := range points {
		pointsMap[point.location.ToRect(tol).String()] = point
		rt.Insert(point)
	}
	return rt, pointsMap
}

func initServer(r *mux.Router) {
	srv := &http.Server{
		Addr:         "0.0.0.0:8080",
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      r,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Println(err)
	}
}

func GetLatLngFromParams(latlngStr string) (error, float64, float64) {
	latlng := strings.Split(latlngStr, ",")
	latStr := latlng[0]
	lngStr := latlng[1]
	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		return err, 0, 0
	}
	lng, err := strconv.ParseFloat(lngStr, 64)
	if err != nil {
		return err, 0, 0
	}
	return nil, lat, lng
}

func GetNearestStops(rt *rtreego.Rtree, point rtreego.Point, pointsMap PointsMap) []RTreePoint {
	results := rt.NearestNeighbors(3, point)
	var resultPoints []RTreePoint
	for _, result := range results {
		rect := result.Bounds()
		resultPoints = append(resultPoints, pointsMap[rect.String()])
	}
	return resultPoints
}

func GetRoutesHandler(stops []Stop, rt *rtreego.Rtree, pointsMap PointsMap) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		latlngStr := params["latlng1"]
		if latlngStr == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		err, lat1, lng1 := GetLatLngFromParams(params["latlng1"])
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		err, lat2, lng2 := GetLatLngFromParams(params["latlng2"])
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fromPoint := rtreego.Point{lat1, lng1}
		toPoint := rtreego.Point{lat2, lng2}
		fromStops := GetNearestStops(rt, fromPoint, pointsMap)
		toStops := GetNearestStops(rt, toPoint, pointsMap)
		data, err := json.Marshal(map[string]string{
			"from stop 1": fromStops[0].stop.String(),
			"from stop 2": fromStops[1].stop.String(),
			"to stop 1":   toStops[0].stop.String(),
			"to stop 2":   toStops[1].stop.String(),
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

func (s Stop) String() string {
	return s.Name
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
