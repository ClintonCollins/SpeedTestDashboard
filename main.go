package main

import (
	"time"
	"os"
	"strings"
	"fmt"
	"os/signal"
	"path/filepath"
	"net/http"
	"github.com/pressly/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/flosch/pongo2"
	"sync"
	"os/exec"
	"encoding/json"
	"sort"
	"github.com/getlantern/systray"
	"github.com/getlantern/systray/example/icon"
	"syscall"
	"github.com/gonutz/w32"
)

var (
	workDir, _ = os.Getwd()
	Closer = make(chan os.Signal, 1)
	SpeedTests []TestGroup
	SpeedTestsLock = sync.RWMutex{}
	ServerIDs = []string{"16683", "3501", "11207"}
)

type TestGroup struct {
	Date time.Time
	Tests []SpeedTestJSON
}

type SpeedTestJSON struct {
	Client struct {
		Ispdlavg  string `json:"ispdlavg"`
		IP        string `json:"ip"`
		Loggedin  string `json:"loggedin"`
		Lon       string `json:"lon"`
		Lat       string `json:"lat"`
		Ispulavg  string `json:"ispulavg"`
		Isprating string `json:"isprating"`
		Isp       string `json:"isp"`
		Rating    string `json:"rating"`
		Country   string `json:"country"`
	} `json:"client"`
	Timestamp     time.Time `json:"timestamp"`
	BytesReceived int       `json:"bytes_received"`
	Upload        float64   `json:"upload"`
	BytesSent     int       `json:"bytes_sent"`
	Share         string    `json:"share"`
	Server        struct {
		Host    string  `json:"host"`
		Cc      string  `json:"cc"`
		Lon     string  `json:"lon"`
		Lat     string  `json:"lat"`
		URL     string  `json:"url"`
		Latency float64 `json:"latency"`
		ID      string  `json:"id"`
		Name    string  `json:"name"`
		D       float64 `json:"d"`
		Country string  `json:"country"`
		Sponsor string  `json:"sponsor"`
	} `json:"server"`
	Ping     float64 `json:"ping"`
	Download float64 `json:"download"`
}

func GracefulExit() {
	<- Closer
	outputLog("Shutting down...")
	shutdown()
	os.Exit(0)
}

// Initializes the database and anything else that is needed on startup \\
func init() {
	loadTestsFromFile()
}

// Called right before a graceful exit \\
func shutdown() {
	saveTestsToFile()
}

func sysTrayShutdown() {
	Closer <- syscall.SIGTERM
}

// Start go routines that will constantly run with the application \\
func startupRoutines() {
	go GracefulExit()
	go systray.Run(onReady, sysTrayShutdown)
	go periodicTestSave()
	go runSpeedTests()
}

func onClickQuit(channel chan struct{}) {
	<-channel
	systray.Quit()
}

func onClickShowConsole(channel chan struct{}) {
	for {
		<-channel
		showConsole()
	}
}

func onClickHideConsole(channel chan struct{}) {
	for {
		<-channel
		hideConsole()
	}
}

func runSpeedTests() {
	rate := time.Minute * 60
	throttle := time.Tick(rate)
	for {
		var SpeedTestGroup []SpeedTestJSON
		var testGroup TestGroup
		for _, id := range ServerIDs {
			outputJson := getSpeedResults(id)
			SpeedTestGroup = append(SpeedTestGroup, outputJson)
		}
		testGroup.Tests = SpeedTestGroup
		testGroup.Date = time.Now()
		SpeedTestsLock.Lock()
		SpeedTests = append(SpeedTests, testGroup)
		SpeedTestsLock.Unlock()
		<-throttle
	}
}

func getSpeedResults(serverID string) SpeedTestJSON {
	outputLog("Running test on server: " + serverID)
	cmd := exec.Command("speedtest-cli", "--json",  "--share", "--server", serverID)
	output, err := cmd.Output()
	if err != nil {
		fmt.Println(err)
	}
	outputJson := SpeedTestJSON{}
	jsonErr := json.Unmarshal(output, &outputJson)
	if jsonErr != nil {
		fmt.Println(jsonErr)
	}
	msg := fmt.Sprintf("Test results are - Ping: %.0fms, Download: %s Mbps, and Upload: %s Mbps From: %s", outputJson.Ping, calcMbps(outputJson.Download), calcMbps(outputJson.Upload), outputJson.Server.Sponsor)
	outputLog(msg)
	return outputJson
}

func calcMbps(bytes float64) string {
	calcSize := bytes / 131072 / 8
	calcSizeFormatted := fmt.Sprintf("%.2f", calcSize)
	return calcSizeFormatted
}

func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit URL parameters.")
	}

	fs := http.StripPrefix(path, http.FileServer(root))

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.ServeHTTP(w, r)
	}))
}

func index(w http.ResponseWriter, r *http.Request) {
	t := pongo2.Must(pongo2.FromFile("templates/index.html"))
	SpeedTestsLock.Lock()
	sort.Slice(SpeedTests, func(i, j int) bool {
		return SpeedTests[i].Date.After(SpeedTests[j].Date)
	})
	Tests := SpeedTests
	if len(Tests) > 24 {
		Tests = Tests[:24]
	}
	SpeedTestsLock.Unlock()
	templateArgs := map[string]interface{}{
		"SpeedTests": Tests,
	}
	t.ExecuteWriter(templateArgs, w)
}

func onReady() {
	systray.SetIcon(icon.Data)
	systray.SetTitle("Speed Tests Dashboard")
	systray.SetTooltip("Speedtest Dashboard")
	mQuit := systray.AddMenuItem("Quit", "Quit the whole app")
	mShow := systray.AddMenuItem("Show Console", "Show Console")
	mHide := systray.AddMenuItem("Hide Console", "Hide Console")
	go onClickQuit(mQuit.ClickedCh)
	go onClickShowConsole(mShow.ClickedCh)
	go onClickHideConsole(mHide.ClickedCh)
}

func hideConsole() {
	console := w32.GetConsoleWindow()
	if console == 0 {
		return // no console attached
	}

	_, consoleProcID := w32.GetWindowThreadProcessId(console)
	if w32.GetCurrentProcessId() == consoleProcID {
		w32.ShowWindowAsync(console, w32.SW_HIDE)
	}
}

func showConsole() {
	console := w32.GetConsoleWindow()
	if console == 0 {
		return // no console attached
	}

	_, consoleProcID := w32.GetWindowThreadProcessId(console)
	if w32.GetCurrentProcessId() == consoleProcID {
		w32.ShowWindowAsync(console, w32.SW_SHOW)
	}
}

func outputLog(message string) {
	timestamp := time.Now().Format("Jan 02 15:04:05")
	botOutput := fmt.Sprintf("[%s]: %s", timestamp, message)
	fmt.Println(botOutput)
}

func main() {
	listenAddress := fmt.Sprintf(":7000")
	signal.Notify(Closer, os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	startupRoutines()
	outputLog("Hiding console")
	hideConsole()
	router := chi.NewRouter()

	// MIDDLEWARE //
	router.Use(middleware.Timeout(60 * time.Second))

	// ROUTING GET //
	router.Get("/", index)

	// EXECUTION //
	outputLog("Finished startup...")
	filesDir := filepath.Join(workDir, "static")
	FileServer(router, "/static", http.Dir(filesDir))
	http.ListenAndServe(listenAddress, router)
}

