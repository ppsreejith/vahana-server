package main

import (
	"encoding/json"
	"errors"
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
	stopMap := LoadStops("./resources/stops.json")
	toRouteMap := LoadPaths("./resources/to-graph.json")
	fromRouteMap := LoadPaths("./resources/from-graph.json")
	routeStopMap := LoadRoutes("./resources/routes.json")
	timetable := LoadTimetable("./resources/timetable.json")
	invertedTimetable := LoadInvertedTimetable("./resources/inverted-timetable.json")
	var vehicleMonitoring VehicleMonitoring
	vehicleMonitoring.GetData()
	rt, pointsMap := createLatLngTree(stopMap)
	r.HandleFunc("/routes/{latlng1}/{latlng2}", GetRoutesHandler(stopMap, rt, pointsMap, toRouteMap, fromRouteMap, routeStopMap, timetable, invertedTimetable, vehicleMonitoring))
	initServer(r)
}

type RTreePoint struct {
	location rtreego.Point
	stop     Stop
}

const tol = 0.001
const MAX_STOPS = 10
const MAX_JOURNEYS = 50

func (s RTreePoint) Bounds() *rtreego.Rect {
	return s.location.ToRect(tol)
}

func GetTime() int {
	return 1556460882000
}

func createLatLngTree(stopMap StopMap) (*rtreego.Rtree, PointsMap) {
	rt := rtreego.NewTree(2, 25, 50)
	points := []RTreePoint{}
	for _, stop := range stopMap {
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

func GetDoubleRouteWholeSigment(fromStop, toStop Stop, toRouteMap Route, fromRouteMap Route, stopMap StopMap) [][]RouteSegment {
	routeSegments := [][]RouteSegment{}
	fromStopNeighborsMap, ok := toRouteMap[fromStop.Name]
	if !ok {
		return routeSegments
	}
	toStopNeighborsMap, ok := fromRouteMap[toStop.Name]
	if !ok {
		return routeSegments
	}
	for fromStopNeighborName, fromPaths := range fromStopNeighborsMap {
		toPaths, ok := toStopNeighborsMap[fromStopNeighborName]
		if !ok {
			continue
		}
		fromStopNeighbor, ok := stopMap[fromStopNeighborName]
		if !ok {
			continue
		}
		for _, fromPath := range fromPaths {
			for _, toPath := range toPaths {
				if fromPath.RouteCode == toPath.RouteCode {
					continue
				}
				routeSegments = append(routeSegments, []RouteSegment{
					RouteSegment{
						FromStop:  fromStop,
						ToStop:    fromStopNeighbor,
						RoutePath: fromPath,
					},
					RouteSegment{
						FromStop:  fromStopNeighbor,
						ToStop:    toStop,
						RoutePath: toPath,
					},
				})
			}
		}
	}
	return routeSegments
}

func GetSingleRouteWholeSigment(fromStop, toStop Stop, toRouteMap Route) []RouteSegment {
	var routeSegments []RouteSegment
	neighborsMap, ok := toRouteMap[fromStop.Name]
	if !ok {
		return routeSegments
	}
	paths, ok := neighborsMap[toStop.Name]
	if !ok {
		return routeSegments
	}
	for _, path := range paths {
		routeSegments = append(routeSegments, RouteSegment{
			FromStop:  fromStop,
			ToStop:    toStop,
			RoutePath: path,
		})
	}
	return routeSegments
}

func GetRouteSegments(segment RouteSegment, routeStopMap RouteStopMap) []RouteSegment {
	routeCode := segment.RoutePath.RouteCode
	routeStops := routeStopMap[routeCode]
	routeSegments := []RouteSegment{}
	var flag = 0

	for index, stop := range routeStops {
		if index == (len(routeStops) - 1) {
			break
		}
		nextStop := routeStops[index+1]
		if stop.Name == segment.ToStop.Name {
			break
		}
		if stop.Name == segment.FromStop.Name {
			flag = 1
		}
		if flag == 1 {
			routeSegments = append(routeSegments, RouteSegment{
				FromStop: stop,
				ToStop:   nextStop,
				RoutePath: RoutePath{
					RouteCode: routeCode,
					Distance:  stop.NextDistance,
				},
			})
		}
	}
	return routeSegments
}

func GetRouteJourneys(fromStops, toStops []Stop, toRouteMap Route, fromRouteMap Route, stopMap StopMap, routeStopMap RouteStopMap, timetable BusStopArrival, invertedTimetable VehicleAtStopAtRoute) []RouteJourney {
	routeJourneys := []RouteJourney{}
	for _, fromStop := range fromStops {
		for _, toStop := range toStops {
			routeWholeSegments := GetSingleRouteWholeSigment(fromStop, toStop, toRouteMap)
			for _, routeWholeSegment := range routeWholeSegments {
				err, vehicleTime := invertedTimetable.GetNearestVehicle(routeWholeSegment.FromStop.Name, routeWholeSegment.RoutePath.RouteCode, GetTime())
				if err != nil {
					continue
				}
				segments := GetRouteSegments(routeWholeSegment, routeStopMap)
				totalDistance := float64(0)
				for _, segment := range segments {
					totalDistance = totalDistance + segment.RoutePath.Distance
				}
				_, timeToArrive := timetable.GetTimeOfArrival(vehicleTime.Vehicle, routeWholeSegment.ToStop.Name)
				journeyTime := timeToArrive - GetTime()
				if journeyTime < 0 {
					journeyTime = -journeyTime
				}
				routeJourneys = append(routeJourneys, RouteJourney{
					Segments:      segments,
					TotalDistance: totalDistance,
					Vehicles:      []VehicleTime{*vehicleTime},
					TotalTime:     journeyTime,
				})
			}
			routeWholeDoubleSegments := GetDoubleRouteWholeSigment(fromStop, toStop, toRouteMap, fromRouteMap, stopMap)
			for _, routeWholeDoubleSegment := range routeWholeDoubleSegments {
				routeSegments := []RouteSegment{}
				var erroredOut bool
				vehicleTimes := []VehicleTime{}
				time := GetTime()
				for _, routeWholeSegment := range routeWholeDoubleSegment {
					err, vehicleTime := invertedTimetable.GetNearestVehicle(routeWholeSegment.FromStop.Name, routeWholeSegment.RoutePath.RouteCode, time)
					if err != nil || erroredOut {
						erroredOut = true
						continue
					}
					err, time = timetable.GetTimeOfArrival(vehicleTime.Vehicle, routeWholeSegment.ToStop.Name)
					if err != nil || erroredOut {
						erroredOut = true
						continue
					}
					vehicleTimes = append(vehicleTimes, *vehicleTime)
					routeSegmentParts := GetRouteSegments(routeWholeSegment, routeStopMap)
					for _, routeSegmentPart := range routeSegmentParts {
						routeSegments = append(routeSegments, routeSegmentPart)
					}
				}
				if erroredOut {
					continue
				}
				totalDistance := float64(0)
				for _, segment := range routeSegments {
					totalDistance = totalDistance + segment.RoutePath.Distance
				}
				journeyTime := time - GetTime()
				if journeyTime < 0 {
					journeyTime = -journeyTime
				}
				routeJourneys = append(routeJourneys, RouteJourney{
					Segments:      routeSegments,
					TotalDistance: totalDistance,
					Vehicles:      vehicleTimes,
					TotalTime:     journeyTime,
				})
			}
		}
	}
	sort.Slice(routeJourneys, func(i, j int) bool {
		return routeJourneys[i].TotalTime < routeJourneys[j].TotalTime
	})
	if len(routeJourneys) > MAX_JOURNEYS {
		return routeJourneys[:MAX_JOURNEYS]
	}
	return routeJourneys
}

func GetRoutesHandler(stopMap StopMap, rt *rtreego.Rtree, pointsMap PointsMap, toRouteMap Route, fromRouteMap Route, routeStopMap RouteStopMap, timetable BusStopArrival, invertedTimetable VehicleAtStopAtRoute, vehicleMonitoring VehicleMonitoring) http.HandlerFunc {
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
		routes := GetRouteJourneys(fromStops, toStops, toRouteMap, fromRouteMap, stopMap, routeStopMap, timetable, invertedTimetable)
		data, err := json.Marshal(map[string][]RouteJourney{
			"journeys": routes,
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
	Name         string   `json:"StopPointName"`
	Location     Position `json:"Location"`
	LineRef      string   `json:"LineRef"`
	Order        float64  `json:"Order"`
	NextDistance float64  `json:"nextDistance"`
}

type StopMap map[string]Stop

type RoutePath struct {
	RouteCode string  `json:"route_code"`
	Distance  float64 `json:"distance"`
}

type RouteSegment struct {
	FromStop  Stop
	ToStop    Stop
	RoutePath RoutePath
}

type RouteJourney struct {
	Segments            []RouteSegment
	TotalDistance       float64
	OverallComfortLevel float64
	Vehicles            []VehicleTime
	TotalTime           int
}

type VehicleInfo struct {
}

type RouteStopMap map[string][]Stop

type RouteMap map[string][]RoutePath
type Route map[string]RouteMap

type StopArrival map[string]int
type BusStopArrival map[string]StopArrival

func (b BusStopArrival) GetTimeOfArrival(vehicle string, stop string) (error, int) {
	err := errors.New("Couldn't find")
	stopMap, ok := b[vehicle]
	if !ok {
		return err, 0
	}
	timeOfArrival, ok := stopMap[stop]
	if !ok {
		return err, 0
	}
	return nil, timeOfArrival
}

type VehicleTime struct {
	Time    int
	Vehicle string
}
type VehicleAtStop map[string][]VehicleTime
type VehicleAtStopAtRoute map[string]VehicleAtStop

func (v VehicleAtStopAtRoute) GetNearestVehicle(stop string, route string, time int) (error, *VehicleTime) {
	stopMap, ok := v[route]
	err := errors.New("Couldn't find")
	if !ok {
		return err, nil
	}
	vehicleTimes, ok := stopMap[stop]
	if !ok {
		return err, nil
	}
	index := sort.Search(len(vehicleTimes), func(index int) bool {
		return vehicleTimes[index].Time > time
	})
	if index == len(vehicleTimes) {
		return err, nil
	}
	return nil, &vehicleTimes[index]
}

type Vehicle struct {
	Location    Position `json:"location"`
	Destination string   `json:"destination"`
}

type VehicleMonitoring struct {
	Data map[string]Vehicle
}

func (v *VehicleMonitoring) GetData() {
	configFile, err := os.Open("./resources/vehicles.json")
	defer configFile.Close()
	if err != nil {
		fmt.Println(err.Error())
	}
	jsonParser := json.NewDecoder(configFile)
	vehicleData := make(map[string]Vehicle)
	jsonParser.Decode(&vehicleData)
	v.Data = vehicleData
}

func (fromStop Stop) GetDistance(lat, lng float64) float64 {
	return disfun.HaversineLatLon(
		fromStop.Location.Latitude,
		fromStop.Location.Longitude,
		lat,
		lng,
	)
}

func LoadStops(file string) StopMap {
	configFile, err := os.Open(file)
	defer configFile.Close()
	if err != nil {
		fmt.Println(err.Error())
	}
	jsonParser := json.NewDecoder(configFile)
	var stops []Stop
	jsonParser.Decode(&stops)
	stopMap := make(StopMap)
	for _, stop := range stops {
		stopMap[stop.Name] = stop
	}
	return stopMap
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

func LoadRoutes(file string) RouteStopMap {
	configFile, err := os.Open(file)
	defer configFile.Close()
	if err != nil {
		fmt.Println(err.Error())
	}
	jsonParser := json.NewDecoder(configFile)
	var routemap RouteStopMap
	jsonParser.Decode(&routemap)
	return routemap
}

func LoadTimetable(file string) BusStopArrival {
	configFile, err := os.Open(file)
	defer configFile.Close()
	if err != nil {
		fmt.Println(err.Error())
	}
	jsonParser := json.NewDecoder(configFile)
	var busStopArrival BusStopArrival
	jsonParser.Decode(&busStopArrival)
	return busStopArrival
}

func LoadInvertedTimetable(file string) VehicleAtStopAtRoute {
	configFile, err := os.Open(file)
	defer configFile.Close()
	if err != nil {
		fmt.Println(err.Error())
	}
	jsonParser := json.NewDecoder(configFile)
	var vehicleAtStopAtRoute VehicleAtStopAtRoute
	jsonParser.Decode(&vehicleAtStopAtRoute)
	for _, stopMap := range vehicleAtStopAtRoute {
		for _, vehicleTimes := range stopMap {
			sort.Slice(vehicleTimes, func(i, j int) bool {
				return vehicleTimes[i].Time < vehicleTimes[j].Time
			})
		}
	}
	return vehicleAtStopAtRoute
}
