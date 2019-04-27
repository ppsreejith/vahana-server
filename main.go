package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dhconnelly/rtreego"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jbowles/disfun"
)

type PointsMap = map[string]RTreePoint

func main() {
	r := mux.NewRouter()
	stops := LoadStops("./resources/stops.json")
	route := LoadPaths("./resources/to-graph.json")
	rt, pointsMap := createLatLngTree(stops)
	r.HandleFunc("/routes/{latlng1}/{latlng2}", GetRoutesHandler(stops, rt, pointsMap, route))
	initServer(r)
}

type RTreePoint struct {
	location rtreego.Point
	stop     Stop
}

const tol = 0.001
const MAX_STOPS = 10

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
		Addr:         "0.0.0.0:9999",
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      handlers.CORS()(r),
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

func GetNearestStops(rt *rtreego.Rtree, point rtreego.Point, pointsMap PointsMap) []Stop {
	results := rt.NearestNeighbors(MAX_STOPS, point)
	var stops []Stop
	cache := make(map[string]bool)
	for _, result := range results {
		stop := pointsMap[result.Bounds().String()].stop
		_, ok := cache[stop.Name]
		if !ok {
			cache[stop.Name] = true
			stops = append(stops, stop)
		}
	}
	sort.Slice(stops, func(i, j int) bool {
		dis1 := stops[i].GetDistance(point[0], point[1])
		dis2 := stops[j].GetDistance(point[0], point[1])
		return dis1 < dis2
	})
	return stops
}

func GetRoutesHandler(stops []Stop, rt *rtreego.Rtree, pointsMap PointsMap, route Route) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "application/json")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
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
		data, err := json.Marshal(map[string][]Stop{
			"from stops": fromStops,
			"to stops":   toStops,
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

type RoutePath struct {
	RouteCode string  `json:"route_code"`
	Distance  float64 `json:"distance"`
}

type RouteMap map[string][]RoutePath
type Route map[string]RouteMap

func (fromStop Stop) GetDistance(lat, lng float64) float64 {
	return disfun.HaversineLatLon(
		fromStop.Location.Latitude,
		fromStop.Location.Longitude,
		lat,
		lng,
	)
}

func LoadStops(file string) []Stop {
	configFile, err := os.Open(file)
	defer configFile.Close()
	if err != nil {
		fmt.Println(err.Error())
	}
	jsonParser := json.NewDecoder(configFile)
	var stops []Stop
	jsonParser.Decode(&stops)
	return stops
}

func LoadPaths(file string) Route {
	configFile, err := os.Open(file)
	defer configFile.Close()
	if err != nil {
		fmt.Println(err.Error())
	}
	var route Route
	jsonParser := json.NewDecoder(configFile)
	jsonParser.Decode(&route)
	return route
}
