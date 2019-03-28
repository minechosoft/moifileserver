package main

import (
	"encoding/json"
	"gopkg.in/natefinch/lumberjack.v2"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var ipv4Mask = net.CIDRMask(24, 32)
var ipv6Mask = net.CIDRMask(32, 128)

type ServerConfig struct {
	Port string
}

type HeaderConfig struct {
	ConfigName                 string
	AccessControlAllowOrigin   string
	AccessControlExposeHeaders string
	CacheControl               string
	ETag                       string
	TimingAllowOrigin          string
	XFrontEnd                  string
}

func getLogFileDestination() string {
	// AP documentation states that this environment variable contains the path to the data directory.
	basePath := os.Getenv("DataDir")
	if basePath == "" {
		localPath, err := filepath.Abs(path.Dir(os.Args[0]))
		if err != nil {
			log.Fatal("Could not get the local path.")
		}
		basePath = localPath
	}

	return path.Join(basePath, "logs\\local\\FileServer\\FileServer.log")
}

func setupLogging() {
	log.SetOutput(&lumberjack.Logger{
		Filename:   getLogFileDestination(),
		MaxSize:    10, // megabytes
		MaxBackups: 0,
		MaxAge:     10, //days
		Compress:   false,
	})
}

func getHeaderConfigs(configFileLoc string) []HeaderConfig {
	config, err := ioutil.ReadFile(configFileLoc)
	headerConfigs := make([]HeaderConfig, 0)
	if err == nil {
		err = json.Unmarshal([]byte(config), &headerConfigs)
		if err != nil {
			log.Fatalln("Failed to unmarshal the json header config.")
		}
	}

	return headerConfigs
}

func setHeaderIfValIsNonEmpty(responseWriter http.ResponseWriter, key string, val string) {
	if val != "" {
		responseWriter.Header().Set(key, val)
	}
}

func scrubIPAddressAsStr(inputIPAddr string) string {
	ipAddress := net.ParseIP(inputIPAddr)
	if ipAddress == nil {
		return ""
	}

	if ipAddress.To4() != nil {
		return ipAddress.Mask(ipv4Mask).String()
	} else if ipAddress.To16() != nil {
		return ipAddress.Mask(ipv6Mask).String()
	}

	return ""
}

// Returns true is path contains an /apc/ resource
func hasCustomAPCHeaders(urlPath string) bool {
	urlPathArr := strings.Split(urlPath, "\\")
	urlPathLen := len(urlPathArr)

	// if no match, len == 1
	// the below supports the paths: /apc/xxx.gif and /footprint/apc/xxx.gif
	return (urlPathLen > 1 && urlPathArr[0] == "apc") || (urlPathLen > 2 && urlPathArr[0] == "edge_footprint" && urlPathArr[1] == "apc")
}

func getEdgeNodeShortName(edgeNode string) string {
	edgeNodeSplitArr := strings.Split(edgeNode, "-")

	// if no match, len == 1
	if len(edgeNodeSplitArr) > 1 {
		return edgeNodeSplitArr[len(edgeNodeSplitArr)-1]
	}

	return ""
}

func addHeadersToHandler(h http.Handler, headerConfigMap map[string]HeaderConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		urlPath := strings.Trim(filepath.Clean(string(r.URL.Path)), "\\")

		headerConfig, hasConfig := headerConfigMap[urlPath]
		if hasConfig {

			// Add headers to the response, if applicable
			setHeaderIfValIsNonEmpty(w, "Access-Control-Allow-Origin", headerConfig.AccessControlAllowOrigin)
			setHeaderIfValIsNonEmpty(w, "Access-Control-Expose-Headers", headerConfig.AccessControlExposeHeaders)
			setHeaderIfValIsNonEmpty(w, "Cache-Control", headerConfig.CacheControl)
			setHeaderIfValIsNonEmpty(w, "ETag", headerConfig.ETag)
			setHeaderIfValIsNonEmpty(w, "Timing-Allow-Origin", headerConfig.TimingAllowOrigin)

			if hasCustomAPCHeaders(urlPath) {
				setHeaderIfValIsNonEmpty(w, "X-FrontEnd", headerConfig.XFrontEnd)
				setHeaderIfValIsNonEmpty(w, "X-EndPoint", getEdgeNodeShortName(string(r.Header.Get("X-FD-EdgeEnvironment"))))
				setHeaderIfValIsNonEmpty(w, "X-UserHostAddress", scrubIPAddressAsStr(string(r.Header.Get("X-FD-SocketIP"))))
			}
		}

		h.ServeHTTP(w, r)
	}
}

func main() {

	setupLogging()

	config, err := ioutil.ReadFile("config.json")
	if err != nil {
		log.Fatalln("Failed to load the config file.")
		os.Exit(1)
	}

	var serverConfig ServerConfig
	err = json.Unmarshal([]byte(config), &serverConfig)
	if err != nil {
		log.Fatalln("Failed to unmarshal the json config.")
	}

	log.Printf("Server Config: %v", serverConfig)
	headerConfigMap := make(map[string]HeaderConfig)
	headerConfigs := getHeaderConfigs("headerConfig.json")

	for _, headerConfig := range headerConfigs {
		headerConfigMap[headerConfig.ConfigName] = headerConfig
	}

	// Add headers to FileServer
	sHandler := addHeadersToHandler(http.FileServer(http.Dir("./files")), headerConfigMap)

	s := &http.Server{
		Addr:           serverConfig.Port,
		Handler:        sHandler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	// Start the HTTP server
	log.Fatal(s.ListenAndServe())
}
