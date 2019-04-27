package main

import (
    "fmt"
    "os"
    "encoding/json"
    "net/http"
    "time"
    "github.com/gorilla/mux"
    "github.com/gorilla/handlers"

)

func main() {
    r := mux.NewRouter()
    fmt.Println("Hello world");
    routemap := LoadRoutes("./resources/routes.json")
    // fmt.Println(routemap)
    r.HandleFunc("/routemapping/{route_id}/{stop1}/{stop2}", GetRoutesHandler(routemap))
    initServer(r)
}

func GetRoutesHandler(routemap RouteMap) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// w.Header().Set("Access-Control-Allow-Origin", "*")
		// w.Header().Set("Access-Control-Allow-Headers", "application/json")
		// w.Header().Set("Access-Control-Allow-Credentials", "true")
		// w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		// w.Header().Set("Content-Type", "application/json")
		// w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		params := mux.Vars(r)
		routeID, _ := params["route_id"]
		stop1, _ := params["stop1"]
		stop2, _ := params["stop2"]
		
        selectedRoute := routemap[routeID]
        var filteredRoute []Stop
        var flag = 0

        for _, stop := range selectedRoute {
            if stop.StopPointName==stop1{
                flag = 1
            }
           
            if flag==1{
                filteredRoute = append(filteredRoute,  stop)
            }
            if stop.StopPointName==stop2{
                flag = 0
            }
        }



        fmt.Println(filteredRoute)

        w.WriteHeader(http.StatusOK)
        data, _ := json.Marshal(filteredRoute)
		w.Write(data)
	}
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
		fmt.Println(err)
	}
}



type Position struct {
	Longitude float64 `json:"Longitude"`
	Latitude  float64 `json:"Latitude"`
}

type Stop struct {
	StopPointName  string   `json:"StopPointName"`
    Location Position `json:"Location"`
    LineRef string `json:"LineRef"`
    Order float64 `json:"Order"`
    NextDistance float64 `json:"nextDistance"`
}

type RouteMap map[string][]Stop

func LoadRoutes(file string) RouteMap {
    configFile, err := os.Open(file)
    defer configFile.Close()
    if err != nil {
        fmt.Println(err.Error())
    }
    jsonParser := json.NewDecoder(configFile)
    var routemap RouteMap
    jsonParser.Decode(&routemap)
    return routemap
}