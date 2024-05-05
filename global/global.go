package global

import (
	"encoding/json"
	"io"
	"os"
	"regexp"
	"runtime"
	"sniper/files"
	"sniper/logger"
	"strings"
	"sync"
	"time"

	"golang.org/x/exp/slices"
)

var (
	ShouldKill bool

	Config             ConfigStruct
	DiscordBuildNumber int
	SnipingToken       string

	LoadedAlts      uint64
	LoadedServers   uint64
	FoundMessages   uint64
	FoundInvites    uint64
	FoundPromocodes uint64
	TotalAlts       uint64
	DeadAlts        uint64
	TotalInvalid    uint64
	TotalMissed     uint64
	TotalClaimed    uint64
	TotalAttempts   uint64

	// API Stats stuff: every 30 seconds we send those and clear the data
	APIStatsMessages uint64
	APIStatsInvites  uint64

	// Stuff that we append to the file every minute
	Invites    []string
	Promocodes []string

	TokenRegex        = regexp.MustCompile(`(mfa.[\w-]{84}|[\w-]{23,24}.[\w-]{6}.[\w-]{38}|[\w-]{24}.[\w-]{6}.[\w-]{27}|[\w-]{26}.[\w-]{6}.[\w-]{38}|[\w-]{24}.[\w-]{5}.([\w-]{38}|[\w-]{37}))`)
	Hostname          string
	MemoryStats       runtime.MemStats
	SpamDetectorPtr   = NewSpamDetector()
	QueueFunctionsPtr = NewQueueFunctions()

	// Received by the API on login
	UserDiscordID  string = ""
	UserName       string = ""
	HasOTCBypass   bool   = false
	DetectedNitros []string
)

func GetConfigAltsStatus() string {
	status := strings.TrimSpace(strings.ToLower(Config.Alts.Status))
	// "unknown" works too
	if status == "idle" ||
		status == "dnd" ||
		status == "online" ||
		status == "offline" ||
		status == "invisible" {
		return status
	}

	return "invisible"
}

func LoadConfig() error {
	jsonFile, err := os.Open("data/config.json")
	if err != nil {
		var temp int = 4

		Config.License.Key = ""
		Config.License.Anonymous = false
		Config.Discord.APIVersion = "10"
		Config.Discord.HostSelection = &temp
		Config.Discord.Webhooks.Successful = ""
		Config.Discord.Webhooks.Missed = ""
		Config.Discord.Webhooks.Updates = ""
		Config.Discord.Webhooks.EmbedMedia = ""
		Config.Sniper.SnipeType = 1
		Config.Sniper.SaveInvites = true
		Config.Sniper.SavePromoCodes = true
		Config.Sniper.SnipeOnMain = false
		Config.Sniper.Threads = 10
		Config.Alts.Status = "invisible" // Either "idle", "dnd", "online", or "offline" https://discord.com/developers/docs/topics/gateway-events#presence-update
		Config.Alts.ForceStatus = false

		js, _ := json.MarshalIndent(Config, "", "\t")

		_ = os.Mkdir("data", os.ModePerm)
		files.CreateFileIfNotExists("data/config.json")
		files.OverwriteFile("data/config.json", string(js))

		logger.Warn("Created an exemplary config.json")
		return err
	}

	defer jsonFile.Close()

	byteValue, err := io.ReadAll(jsonFile)
	if err != nil {
		return err
	}

	err = json.Unmarshal(byteValue, &Config)
	return err
}

type ConfigStruct struct {
	License struct {
		Key       string `json:"key"`
		Anonymous bool   `json:"anonymous"`
	} `json:"license"`
	Discord struct {
		APIVersion    string `json:"apiVersion"`
		HostSelection *int   `json:"hostSelection"`
		Webhooks      struct {
			Successful string `json:"successful"`
			Missed     string `json:"missed"`
			Updates    string `json:"updates"`
			EmbedMedia string `json:"embedMedia"`
		} `json:"webhooks"`
	} `json:"discord"`
	Sniper struct {
		SnipeType      int  `json:"snipeType"`
		SaveInvites    bool `json:"saveInvites"`
		SnipeOnMain    bool `json:"snipeOnMain"`
		SavePromoCodes bool `json:"savePromoCodes"`
		Threads        int  `json:"threads"`
	} `json:"sniper"`
	Alts struct {
		Status      string `json:"status"`
		ForceStatus bool   `json:"forceStatus"`
	} `json:"alts"`
}

func HideTokenLog(token string) string {
	split := strings.Split(token, ".")
	if len(split) != 3 {
		if len(token) > 5 {
			return token[len(token)-5:]
		}

		return token
	}

	return split[0] + ".." + split[2][len(split[2])-5:]
}

func ProcessToken(rawToken string) string {
	token := TokenRegex.FindStringSubmatch(rawToken)
	if len(token) > 0 && len(token[0]) > 0 {
		return token[0]
	}

	return ""
}

// oh yes
var altTokensMutex sync.Mutex

func ParseAlts() ([]string, error) {
	altTokensMutex.Lock()
	defer altTokensMutex.Unlock()

	files.CreateFileIfNotExists("data/alts.txt")
	Tokens, err := files.ReadLines("data/alts.txt")
	if err != nil {
		return nil, err
	}

	allKeys := make(map[string]bool)
	list := []string{}
	for _, item := range Tokens {
		token := ProcessToken(item)
		if len(token) > 0 {
			if _, value := allKeys[token]; !value {
				allKeys[token] = true
				list = append(list, token)
			}
		}
	}

	return list, nil
}

func GetTokenFull(token string) string {
	altTokensMutex.Lock()
	defer altTokensMutex.Unlock()

	Tokens, err := files.ReadLines("data/alts.txt")
	if err != nil {
		return ""
	}

	index := slices.IndexFunc(Tokens, func(e string) bool {
		return ProcessToken(e) == token
	})

	if index == -1 {
		return ""
	}

	return Tokens[index]
}

func RemoveAltToken(tokenRemoveRaw string) {
	tokenRemove := ProcessToken(tokenRemoveRaw)
	if len(tokenRemove) == 0 {
		return
	}

	altTokensMutex.Lock()
	defer altTokensMutex.Unlock()

	Tokens, err := files.ReadLines("data/alts.txt")
	if err != nil {
		return
	}

	index := slices.IndexFunc(Tokens, func(e string) bool {
		return ProcessToken(e) == tokenRemove
	})

	if index == -1 {
		return
	}

	Tokens = append(Tokens[:index], Tokens[index+1:]...)
	files.OverwriteFile("data/alts.txt", strings.Join(Tokens, "\n"))
}

func ParseClaimToken() (string, error) {
	files.CreateFileIfNotExists("data/claimToken.txt")
	Tokens, err := files.ReadLines("data/claimToken.txt")
	if err != nil {
		return "", err
	}

	for _, item := range Tokens {
		token := ProcessToken(item)
		if len(token) > 0 {
			return token, nil
		}
	}

	return "", nil
}

type SpamDetector struct {
	sync.RWMutex
	MessageCount map[string]int

	SpamThreshold int           // Maximum number of messages allowed within the time frame
	TimeFrame     time.Duration // Time frame for counting messages
}

func NewSpamDetector() *SpamDetector {
	return &SpamDetector{
		MessageCount:  make(map[string]int),
		SpamThreshold: 5,
		TimeFrame:     time.Minute,
	}
}

func (d *SpamDetector) GetCounter(identifier string) int {
	d.RLock()
	defer d.RUnlock()

	return d.MessageCount[identifier]
}

func (d *SpamDetector) IncrementCounter(identifier string) int {
	d.Lock()
	defer d.Unlock()

	d.MessageCount[identifier]++

	go func() {
		// Schedule the count decrement after the time frame
		time.AfterFunc(d.TimeFrame, func() {
			d.Lock()
			defer d.Unlock()

			// Decrement the count for the user
			if d.MessageCount[identifier] > 0 {
				d.MessageCount[identifier]--
			}
		})
	}()

	return d.MessageCount[identifier]
}

func (d *SpamDetector) IsSpam(identifier string) bool {
	count := d.GetCounter(identifier)

	// create a routine that's going to increment the counter
	go d.IncrementCounter(identifier)

	// Check if count exceeds the spam threshold
	return count > d.SpamThreshold
}

type queueFunctionFnType struct {
	Function  func(...any)
	Routine   bool
	Arguments []any
}

type QueueFunctions struct {
	lock sync.Mutex
	cond *sync.Cond

	queueSlice []queueFunctionFnType
	delay      time.Duration

	closed bool
}

func NewQueueFunctions() *QueueFunctions {
	var ret = &QueueFunctions{}
	ret.cond = sync.NewCond(&ret.lock)

	return ret
}

func (d *QueueFunctions) Init(workers int, delay time.Duration) {
	d.delay = delay
	for i := 0; i < workers; i++ {
		go d.workerRoutine()
	}
}

func (d *QueueFunctions) Close() {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.closed = true
	d.cond.Broadcast()
}

func (d *QueueFunctions) IsClosed() bool {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.closed
}

func (d *QueueFunctions) Queue(runRoutine bool, fn func(...any), args ...any) {
	d.cond.L.Lock()
	defer d.cond.L.Unlock()

	d.queueSlice = append(d.queueSlice, queueFunctionFnType{
		Function:  fn,
		Routine:   runRoutine,
		Arguments: args,
	})

	d.cond.Broadcast()
}

// returns d.closed
func (d *QueueFunctions) work() bool {
	d.cond.L.Lock()

	for len(d.queueSlice) == 0 {
		if d.closed {
			return true
		}

		d.cond.Wait()
	}

	fn := d.queueSlice[0]
	d.queueSlice = d.queueSlice[1:]
	d.cond.L.Unlock()

	if fn.Routine {
		go fn.Function(fn.Arguments...)
	} else {
		fn.Function(fn.Arguments...)
	}

	return false
}

func (d *QueueFunctions) workerRoutine() {
	for {
		if d.work() {
			return
		}

		if d.delay > 0 {
			time.Sleep(d.delay)
		}
	}
}
