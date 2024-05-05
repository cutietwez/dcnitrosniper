package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sniper/discows"
	filelimit "sniper/file_limit"
	"sniper/files"
	"sniper/global"
	"sniper/logger"
	"sniper/request"
	"sniper/sniper"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/radovskyb/watcher"
)

var (
	SniperList []*sniper.Sniper
)

func FreezeApp() {
	logger.Info("Press CTRL+C to kill the app.")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}

func UpdateSnipingToken() {
	newSnipingToken, err := global.ParseClaimToken()
	if err != nil {
		return
	}

	if newSnipingToken == "" {
		return
	}

	lastSnipingToken := global.SnipingToken
	global.SnipingToken = newSnipingToken

	if global.SnipingToken != lastSnipingToken {
		request.OnClaimTokenChange(global.SnipingToken)
		logger.Info("Omg what a fucking joke, New main alt!", logger.FieldString("old", global.HideTokenLog(lastSnipingToken)), logger.FieldString("new", global.HideTokenLog(global.SnipingToken)))
	}
}

func formatNumber(number int64) string {
	in := strconv.FormatInt(number, 10)
	//in := strconv.Itoa(number)
	out := make([]byte, len(in)+(len(in)-2+int(in[0]/'0'))/3)

	if in[0] == '-' {
		in, out[0] = in[1:], '-'
	}

	for i, j, k := len(in)-1, len(out)-1, 0; ; i, j = i-1, j-1 {
		out[j] = in[i]

		if i == 0 {
			return string(out)
		}

		if k++; k == 3 {
			j, k = j-1, 0
			out[j] = ','
		}
	}
}

func main() {
	logger.PrintLogo(false)
	_ = filelimit.SetFileLimit()

	// create the shit so the user knows..
	_ = os.Mkdir("data", os.ModePerm)
	files.CreateFileIfNotExists("data/claimToken.txt")
	files.CreateFileIfNotExists("data/alts.txt")

	err := global.LoadConfig()
	if err != nil {
		logger.Error("Failed to load config", logger.FieldAny("error", err))
		return
	}

	if global.Config.Sniper.Threads < 1 {
		logger.Error("You have less than one thread in config. Please put AT LEAST one thread.")
		return
	}

	global.Hostname, err = os.Hostname()
	if err != nil {
		global.Hostname = "Unknown"
		err = nil
	}

	// get claim token
	global.SnipingToken, err = global.ParseClaimToken()
	if err != nil {
		logger.Error("Error parsing claimToken", logger.FieldString("path", "data/claimToken.txt"), logger.FieldAny("error", err))
		return
	}

	if len(global.SnipingToken) <= 0 {
		logger.Error("Please input a claimToken", logger.FieldString("path", "data/claimToken.txt"))
		return
	}

	alts, err := global.ParseAlts()
	if err != nil {
		logger.Error("Error parsing alts", logger.FieldString("path", "data/alts.txt"), logger.FieldAny("error", err))
		return
	}

	// append claimToken to alts
	if global.Config.Sniper.SnipeOnMain {
		alts = append(alts, global.SnipingToken)
	}

	if len(alts) <= 0 {
		logger.Fail("No alts found")
		return
	}

	//sniper.WebhookFail("test", time.Duration(time.Second), "dasdsa", "asdsa", "asdas", "lol")

	global.LoadedAlts = 0
	atomic.StoreUint64(&global.TotalAlts, uint64(len(alts)))
	logger.Info("Parsed alts", logger.FieldInt("alts", len(alts)))

	// get discord build number
	global.DiscordBuildNumber, err = sniper.GetDiscordBuildNumber()
	if err != nil {
		logger.Error("Failed to fetch discord build number", logger.FieldAny("error", err))
		return
	}

	if len(strconv.Itoa(global.DiscordBuildNumber)) < 6 {
		logger.Error("Failed to get discord build number", logger.FieldInt("parsed", global.DiscordBuildNumber), logger.FieldString("error", "unknown"))
		return
	}

	logger.Info("Fetched Discord Data", logger.FieldInt("build", global.DiscordBuildNumber))

	// initialize request
	var userAgent string = fmt.Sprintf("Discord/%d-IS/%s-%d", global.DiscordBuildNumber, strings.Split(global.Config.License.Key, "-")[3], time.Now().UnixNano())
	request.Init(userAgent, global.SnipingToken)

	// create a routine that will constantly update claim token
	go func() {
		// for {
		// 	UpdateSnipingToken()
		// }

		w := watcher.New()

		go func() {
			for {
				select {
				case event := <-w.Event:
					_ = event

					UpdateSnipingToken()
				case err := <-w.Error:
					fmt.Println(err)
				case <-w.Closed:
					return
				}
			}
		}()

		if err := w.Add("./data/claimToken.txt"); err != nil {
			return
		}

		go func() { w.Wait() }()
		if err := w.Start(time.Millisecond * 1000); err != nil {
			return
		}
	}()

	logger.Info("Starting sniper")

	// goroutine: waits for the app to stop and handle stuff
	go func() {
		// hide the cursor for now. we will show it again when we stop the app
		logger.HideTerminalCursor()

		stopChan := make(chan os.Signal, 1)
		signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)
		<-stopChan

		// show the cursor now.
		logger.ShowTerminalCursor()

		global.ShouldKill = true
		go global.QueueFunctionsPtr.Close()

		fmt.Println()
		logger.Info("Quitting..")

		for _, sniper := range SniperList {
			sniper.Close()
		}

		time.Sleep(time.Second)
		fmt.Println()

		go os.Exit(0)
	}()

	// removed: goroutine: send stats to the API every 30 seconds

	// goroutine: save invites/promocodes every 30 seconds
	if global.Config.Sniper.SaveInvites {
		go func() {
			for !global.ShouldKill {
				time.Sleep(time.Second * 30)

				if len(global.Invites) > 0 {
					files.AppendFile("data/invites.txt", strings.Join(global.Invites, "\n"))
					global.Invites = nil
				}

				if len(global.Promocodes) > 0 {
					files.AppendFile("data/promocodes.txt", strings.Join(global.Promocodes, "\n"))
					global.Promocodes = nil
				}
			}
		}()
	} else {
		logger.Warn("Invites are not being saved fam")
	}

	// goroutine: spinner stuff
	go func() {
		for !global.ShouldKill {
			for _, spinner := range []string{"/", "-", "\\", "|"} {
				bToMb := func(b uint64) uint64 {
					return b / 1024 / 1024
				}

				runtime.ReadMemStats(&global.MemoryStats)

				logger.CallSpinnerTitle(spinner, fmt.Sprintf("Sniping %s servers | %s/%s Alts (%s dead) | %s Messages | %s Invites | %s Attempts | %s Claimed | %s Missed | %s Invalid | %s Promocodes | %s MB/s", formatNumber(int64(atomic.LoadUint64(&global.LoadedServers))), formatNumber(int64(atomic.LoadUint64(&global.LoadedAlts))), formatNumber(int64(atomic.LoadUint64(&global.TotalAlts))), formatNumber(int64(atomic.LoadUint64(&global.DeadAlts))), formatNumber(int64(atomic.LoadUint64(&global.FoundMessages))), formatNumber(int64(atomic.LoadUint64(&global.FoundInvites))), formatNumber(int64(atomic.LoadUint64(&global.TotalAttempts))), formatNumber(int64(atomic.LoadUint64(&global.TotalClaimed))), formatNumber(int64(atomic.LoadUint64(&global.TotalMissed))), formatNumber(int64(atomic.LoadUint64(&global.TotalInvalid))), formatNumber(int64(atomic.LoadUint64(&global.FoundPromocodes))), formatNumber(int64(bToMb(global.MemoryStats.Alloc)))))

				time.Sleep(time.Millisecond * 150)
			}
		}
	}()

	// initialize our function queue, for multi-threading apps
	global.QueueFunctionsPtr.Init(global.Config.Sniper.Threads, time.Millisecond*time.Duration(500*global.Config.Sniper.Threads))

	// let's make our snipers
	for _, token := range alts {
		SniperList = append(SniperList, &sniper.Sniper{
			Token: token,
		})

		global.QueueFunctionsPtr.Queue(false, func(a ...any) {
			var sniperInfo *sniper.Sniper = a[0].(*sniper.Sniper)

			retries := 0

		retryLabel:
			err := sniperInfo.Init()
			if err != nil {
				if err != discows.ErrWSAlreadyOpen {
					if retries == 0 {
						retries++
						time.Sleep(time.Second)
						goto retryLabel
					} else {
						atomic.AddUint64(&global.TotalAlts, ^uint64(0))
						logger.Error("Error initiating sniper", logger.FieldString("token", sniperInfo.Token), logger.FieldAny("error", err))
					}
				}
			}
		}, SniperList[len(SniperList)-1])
	}

	// a goroutine that counts data
	// go func() {
	// 	for !ShouldKill {
	// 		totalServers := 0

	// 		SnipersMutex.Lock()
	// 		for _, v := range SniperList {
	// 			totalServers += v.Guilds
	// 		}
	// 		SnipersMutex.Unlock()

	// 		global.DataMutex.Lock()
	// 		global.LoadedServers = totalServers
	// 		global.DataMutex.Unlock()

	// 		time.Sleep(time.Millisecond * 500)
	// 	}
	// }()

	select {}
}
