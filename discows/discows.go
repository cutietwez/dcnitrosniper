package discows

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"sniper/global"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// Discord WebSocket
// Can be used for self-bot
// Optimized for sniper use

var (
	//wssGatewayURL = "wss://gateway.discord.gg/?encoding=json&v=9&compress=zlib-stream"
	wssGatewayURL = "wss://gateway.discord.gg/?encoding=json&v=9"
)

var ErrWSAlreadyOpen = errors.New("web socket already opened")
var ErrWSNotFound = errors.New("no websocket connection exists")

// Constants for the gateway events
const (
	EventTypeReady string = "READY"
	// EventTypeResumed       string = "RESUMED"
	EventTypeGuildCreate   string = "GUILD_CREATE"
	EventTypeGuildUpdate   string = "GUILD_UPDATE"
	EventTypeGuildDelete   string = "GUILD_DELETE"
	EventTypeMessageCreate string = "MESSAGE_CREATE"
	// EventTypeMessageUpdate string = "MESSAGE_UPDATE"
)

// Opcode are opcodes used by discord
type Opcode int

// https://discord.com/developers/docs/topics/opcodes-and-status-codes#gateway-gateway-opcodes
const (
	OpcodeDispatch            Opcode = iota // Receive
	OpcodeHeartbeat                         // Send/Receive
	OpcodeIdentify                          // Send
	OpcodePresenceUpdate                    // Send
	OpcodeVoiceStateUpdate                  // Send
	_                                       //
	OpcodeResume                            // Send
	OpcodeReconnect                         // Receive
	OpcodeRequestGuildMembers               // Send
	OpcodeInvalidSession                    // Receive
	OpcodeHello                             // Receive
	OpcodeHeartbeatACK                      // Receive
)

// thanks disgoorg/disgo

type CloseEventCode struct {
	Code        int
	Description string
	Explanation string
	Reconnect   bool
}

var (
	CloseEventCodeUnknownError = CloseEventCode{
		Code:        4000,
		Description: "Unknown error",
		Explanation: "We're not sure what went wrong. Try reconnecting?",
		Reconnect:   true,
	}

	CloseEventCodeUnknownOpcode = CloseEventCode{
		Code:        4001,
		Description: "Unknown opcode",
		Explanation: "You sent an invalid Gateway opcode or an invalid payload for an opcode. Don't do that!",
		Reconnect:   true,
	}

	CloseEventCodeDecodeError = CloseEventCode{
		Code:        4002,
		Description: "Decode error",
		Explanation: "You sent an invalid payload to Discord. Don't do that!",
		Reconnect:   true,
	}

	CloseEventCodeNotAuthenticated = CloseEventCode{
		Code:        4003,
		Description: "Not authenticated",
		Explanation: "You sent us a payload prior to identifying.",
		Reconnect:   true,
	}

	CloseEventCodeAuthenticationFailed = CloseEventCode{
		Code:        4004,
		Description: "Authentication failed",
		Explanation: "The account token sent with your identify payload is incorrect.",
		Reconnect:   false,
	}

	CloseEventCodeAlreadyAuthenticated = CloseEventCode{
		Code:        4005,
		Description: "Already authenticated",
		Explanation: "You sent more than one identify payload. Don't do that!",
		Reconnect:   true,
	}

	CloseEventCodeInvalidSeq = CloseEventCode{
		Code:        4007,
		Description: "Invalid seq",
		Explanation: "The sequence sent when resuming the session was invalid. Reconnect and start a new session.",
		Reconnect:   true,
	}

	CloseEventCodeRateLimited = CloseEventCode{
		Code:        4008,
		Description: "Rate limited.",
		Explanation: "You're sending payloads to us too quickly. Slow it down! You will be disconnected on receiving this.",
		Reconnect:   true,
	}

	CloseEventCodeSessionTimed = CloseEventCode{
		Code:        4009,
		Description: "Session timed out",
		Explanation: "Your session timed out. Reconnect and start a new one.",
		Reconnect:   true,
	}

	CloseEventCodeInvalidShard = CloseEventCode{
		Code:        4010,
		Description: "Invalid shard",
		Explanation: "You sent us an invalid shard when identifying.",
		Reconnect:   false,
	}

	CloseEventCodeShardingRequired = CloseEventCode{
		Code:        4011,
		Description: "Sharding required",
		Explanation: "The session would have handled too many guilds - you are required to shard your connection in order to connect.",
		Reconnect:   false,
	}

	CloseEventCodeInvalidAPIVersion = CloseEventCode{
		Code:        4012,
		Description: "Invalid API version",
		Explanation: "You sent an invalid version for the gateway.",
		Reconnect:   false,
	}

	CloseEventCodeInvalidIntent = CloseEventCode{
		Code:        4013,
		Description: "Invalid intent(s)",
		Explanation: "You sent an invalid intent for a Gateway Intent. You may have incorrectly calculated the bitwise value.",
		Reconnect:   false,
	}

	CloseEventCodeDisallowedIntent = CloseEventCode{
		Code:        4014,
		Description: "Disallowed intent(s)",
		Explanation: "You sent a disallowed intent for a Gateway Intent. You may have tried to specify an intent that you have not enabled or are not approved for.",
		Reconnect:   false,
	}

	CloseEventCodeUnknown = CloseEventCode{
		Code:        0,
		Description: "Unknown",
		Explanation: "Unknown Gateway Close Event Code",
		Reconnect:   true,
	}

	CloseEventCodes = map[int]CloseEventCode{
		CloseEventCodeUnknownError.Code:         CloseEventCodeUnknownError,
		CloseEventCodeUnknownOpcode.Code:        CloseEventCodeUnknownOpcode,
		CloseEventCodeDecodeError.Code:          CloseEventCodeDecodeError,
		CloseEventCodeNotAuthenticated.Code:     CloseEventCodeNotAuthenticated,
		CloseEventCodeAuthenticationFailed.Code: CloseEventCodeAuthenticationFailed,
		CloseEventCodeAlreadyAuthenticated.Code: CloseEventCodeAlreadyAuthenticated,
		CloseEventCodeInvalidSeq.Code:           CloseEventCodeInvalidSeq,
		CloseEventCodeRateLimited.Code:          CloseEventCodeRateLimited,
		CloseEventCodeSessionTimed.Code:         CloseEventCodeSessionTimed,
		CloseEventCodeInvalidShard.Code:         CloseEventCodeInvalidShard,
		CloseEventCodeInvalidAPIVersion.Code:    CloseEventCodeInvalidAPIVersion,
		CloseEventCodeInvalidIntent.Code:        CloseEventCodeInvalidIntent,
		CloseEventCodeDisallowedIntent.Code:     CloseEventCodeDisallowedIntent,
	}
)

func CloseEventCodeByCode(code int) CloseEventCode {
	closeCode, ok := CloseEventCodes[code]
	if !ok {
		return CloseEventCodeUnknown
	}
	return closeCode
}

type WSMessage struct {
	Op Opcode          `json:"op,omitempty"`
	D  json.RawMessage `json:"d,omitempty"`
	S  int             `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
}

type HeartBeatMessage int

type IdentifyMessage struct {
	Token        string `json:"token"`
	Capabilities int    `json:"capabilities"`
	Properties   struct {
		Os                     string      `json:"os"`
		Browser                string      `json:"browser"`
		Device                 string      `json:"device"`
		SystemLocale           string      `json:"system_locale"`
		BrowserUserAgent       string      `json:"browser_user_agent"`
		BrowserVersion         string      `json:"browser_version"`
		OsVersion              string      `json:"os_version"`
		Referrer               string      `json:"referrer"`
		ReferringDomain        string      `json:"referring_domain"`
		ReferrerCurrent        string      `json:"referrer_current"`
		ReferringDomainCurrent string      `json:"referring_domain_current"`
		ReleaseChannel         string      `json:"release_channel"`
		ClientBuildNumber      int         `json:"client_build_number"`
		ClientEventSource      interface{} `json:"client_event_source"`
	} `json:"properties"`
	Presence struct {
		Status     string        `json:"status"`
		Since      int           `json:"since"`
		Activities []interface{} `json:"activities"`
		Afk        bool          `json:"afk"`
	} `json:"presence"`
	Compress    bool `json:"compress"`
	ClientState struct {
		GuildVersions            map[string]string `json:"guild_versions"`
		HighestLastMessageID     string            `json:"highest_last_message_id"`
		ReadStateVersion         int               `json:"read_state_version"`
		UserGuildSettingsVersion int               `json:"user_guild_settings_version"`
		UserSettingsVersion      int               `json:"user_settings_version"`
		PrivateChannelsVersion   string            `json:"private_channels_version"`
		APICodeVersion           int               `json:"api_code_version"`
	} `json:"client_state"`
}

type ResumeMessage struct {
	Token     string `json:"token"`
	SessionID string `json:"session_id"`
	Seq       int    `json:"seq"`
}

type HelloMessage struct {
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
}

type PresenceActivity struct {
	Name  string `json:"name"`
	Type  int    `json:"type"`
	State string `json:"state"`
	Emoji struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Animated bool   `json:"animated"`
	} `json:"emoji"`
}

type ReadyMessage struct {
	User             DiscordUser    `json:"user"`
	SessionID        string         `json:"session_id"`
	ResumeGatewayURL string         `json:"resume_gateway_url"`
	Guilds           []DiscordGuild `json:"guilds"`
	Sessions         []struct {
		Status     string             `json:"status"`
		SessionID  string             `json:"session_id"`
		Activities []PresenceActivity `json:"activities"`
		Active     bool               `json:"active,omitempty"`
	} `json:"sessions"`
}

type DiscordGuild struct {
	Properties struct {
		Name string `json:"name"`
	} `json:"properties"`
	ID          string `json:"id"`
	MemberCount int    `json:"member_count"`
}

type DiscordUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator,omitempty"`
}

func (user *DiscordUser) String() string {
	if len(user.Discriminator) > 1 {
		return user.Username + "#" + user.Discriminator
	}

	return user.Username
}

type DiscordMessage struct {
	Content string      `json:"content"`
	Author  DiscordUser `json:"author"`
	GuildID string      `json:"guild_id,omitempty"`
}

type GuildCreate struct {
	DiscordGuild
}

type GuildDelete struct {
	ID string `json:"id"`
}

type GuildUpdate struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ClientCache struct {
	sync.RWMutex

	User                   DiscordUser
	Status                 string
	Activities             []PresenceActivity
	GuildWithMostMembersID string
	guildNames             map[string]string
}

func (cache *ClientCache) Reset() {
	cache.Lock()
	defer cache.Unlock()

	if cache.guildNames != nil {
		cache.guildNames = make(map[string]string)
	}
}

func (cache *ClientCache) Init() {
	cache.Lock()
	defer cache.Unlock()

	if cache.guildNames == nil {
		cache.guildNames = make(map[string]string)
	}
}

func (cache *ClientCache) SetGuildName(guildID, guildName string) {
	cache.Lock()
	defer cache.Unlock()

	if cache.guildNames == nil {
		cache.guildNames = make(map[string]string)
	}

	cache.guildNames[guildID] = guildName
}

func (cache *ClientCache) RemoveGuild(guildID string) {
	cache.Lock()
	defer cache.Unlock()

	if cache.guildNames == nil {
		return
	}

	delete(cache.guildNames, guildID)
}

func (cache *ClientCache) GetGuildName(guildID string) string {
	cache.Lock()
	defer cache.Unlock()

	if cache.guildNames == nil {
		return ""
	}

	if value, contains := cache.guildNames[guildID]; contains {
		return value
	}

	return ""
}

type Client struct {
	sync.RWMutex

	conn       *websocket.Conn
	gateawyURL string

	messagesChan chan interface{}

	heartbeatChan     chan interface{}
	heartbeatInterval time.Duration
	// lastHeartbeatSent time.Time
	// lastHeartbeatReceived time.Time

	LastSequenceReceived int
	SessionID            string

	Token              string
	discordBuildNumber int
	Cache              ClientCache

	OnClose         func(code int, text string) error
	OnReady         func(ready *ReadyMessage)
	OnMessageCreate func(message *DiscordMessage)
}

func NewClient(Token string,
	DiscordBuildNumber int,
	OnClose func(code int, text string) error,
	OnReady func(ready *ReadyMessage),
	OnMessageCreate func(message *DiscordMessage)) *Client {
	return &Client{
		Token:              Token,
		discordBuildNumber: DiscordBuildNumber,
		OnClose:            OnClose,
		OnReady:            OnReady,
		OnMessageCreate:    OnMessageCreate,
	}
}

func (client *Client) Open() error {
	client.Lock()
	defer client.Unlock()

	if client.conn != nil {
		return ErrWSAlreadyOpen
	}

	if client.gateawyURL == "" {
		client.gateawyURL = wssGatewayURL
	}

	if parsedURL, err := url.Parse(client.gateawyURL); err == nil {
		if !strings.HasSuffix(parsedURL.Path, "/") {
			parsedURL.Path = parsedURL.Path + "/"
		}

		query := parsedURL.Query()
		query.Set("encoding", "json")
		query.Set("v", "9")
		parsedURL.RawQuery = query.Encode()

		client.gateawyURL = parsedURL.String()
	}

	// client.lastHeartbeatSent = time.Now().UTC()
	conn, _, err := websocket.DefaultDialer.Dial(client.gateawyURL, nil)
	if err != nil {
		// client.gateawyURL = ""
		return err
	}

	if client.OnClose == nil {
		client.OnClose = func(code int, text string) error {
			return nil
		}
	}

	conn.SetCloseHandler(client.OnClose)

	client.conn = conn

	if client.messagesChan == nil {
		client.messagesChan = make(chan interface{})
	}

	if client.heartbeatChan == nil {
		client.heartbeatChan = make(chan interface{})
	}

	client.Cache.Init()
	go client.listenForMessages(conn)

	return nil
}

func (client *Client) Close() {
	client.CloseWithCode(websocket.CloseNormalClosure)
}

func (client *Client) CloseWithCode(closeCode int) {
	client.Lock()
	defer client.Unlock()

	if closeCode != websocket.CloseServiceRestart {
		client.Cache.Reset()
	}

	if client.messagesChan != nil {
		close(client.messagesChan)
		client.messagesChan = nil
	}

	if client.heartbeatChan != nil {
		close(client.heartbeatChan)
		client.heartbeatChan = nil
	}

	if client.conn != nil {
		_ = client.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(closeCode, ""))
		_ = client.conn.Close()
		client.conn = nil

		if closeCode == websocket.CloseNormalClosure || closeCode == websocket.CloseGoingAway {
			client.SessionID = ""
			client.gateawyURL = ""
			client.LastSequenceReceived = 0
		}
	}
}

func (client *Client) reconnect() {
	global.QueueFunctionsPtr.Queue(true, func(a ...any) {
		var err error

		var wait time.Duration = time.Duration(1)

		for {
			err = client.Open()
			if err == nil {
				return
			}

			// Certain race conditions can call reconnect() twice. If this happens, we
			// just break out of the reconnect loop
			if err == ErrWSAlreadyOpen {
				return
			}

			// don't reconnect if we shouldn't
			if closeError, ok := err.(*websocket.CloseError); ok {
				closeCode := CloseEventCodeByCode(closeError.Code)
				if !closeCode.Reconnect {
					return
				}
			}

			<-time.After(wait * time.Second)
			wait *= 2
			if wait > 600 {
				wait = 600
			}
		}
	})
}

func (client *Client) identifyNew() error {
	identifyData := IdentifyMessage{}
	identifyData.Token = client.Token
	identifyData.Capabilities = 16381

	identifyData.Properties.Os = "Mac OS X" // "Windows"
	identifyData.Properties.Browser = "Chrome"
	identifyData.Properties.Device = ""
	identifyData.Properties.SystemLocale = "en-US"
	identifyData.Properties.BrowserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36" //"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/112.0.0.0 Safari/537.36"
	identifyData.Properties.BrowserVersion = "116.0.0.0"                                                                                                               // "112.0.0.0"
	identifyData.Properties.OsVersion = "10.15.7"                                                                                                                      // "10"
	identifyData.Properties.Referrer = ""
	identifyData.Properties.ReferringDomain = ""
	identifyData.Properties.ReferrerCurrent = ""
	identifyData.Properties.ReferringDomainCurrent = ""
	identifyData.Properties.ReleaseChannel = "stable"
	identifyData.Properties.ClientBuildNumber = client.discordBuildNumber
	identifyData.Properties.ClientEventSource = nil

	identifyData.Presence.Status = "online" // unknown
	identifyData.Presence.Since = 0
	identifyData.Presence.Activities = []interface{}{}
	identifyData.Presence.Afk = false

	identifyData.Compress = false

	identifyData.ClientState.GuildVersions = map[string]string{}
	identifyData.ClientState.HighestLastMessageID = "0"
	identifyData.ClientState.ReadStateVersion = 0
	identifyData.ClientState.UserGuildSettingsVersion = -1
	identifyData.ClientState.UserSettingsVersion = -1
	identifyData.ClientState.PrivateChannelsVersion = "0"
	identifyData.ClientState.APICodeVersion = 0

	return client.SendWSMessage(OpcodeIdentify, identifyData)
}

func (client *Client) resume() error {
	client.RLock()
	token := client.Token
	sessionID := client.SessionID
	lastSequenceReceived := client.LastSequenceReceived
	client.RUnlock()

	return client.SendWSMessage(OpcodeResume, ResumeMessage{
		Token:     token,
		SessionID: sessionID,
		Seq:       lastSequenceReceived,
	})
}

func (client *Client) sendClientData() {
	// {"op":4,"d":{"guild_id":null,"channel_id":null,"self_mute":true,"self_deaf":false,"self_video":false,"flags":2}}
	type voiceStateUpdateData struct {
		GuildID   interface{} `json:"guild_id"`
		ChannelID interface{} `json:"channel_id"`
		SelfMute  bool        `json:"self_mute"`
		SelfDeaf  bool        `json:"self_deaf"`
		SelfVideo bool        `json:"self_video"`
		Flags     int         `json:"flags"`
	}

	client.SendWSMessage(OpcodeVoiceStateUpdate, voiceStateUpdateData{
		GuildID:   nil,
		ChannelID: nil,
		SelfMute:  true,
		SelfDeaf:  false,
		SelfVideo: false,
		Flags:     2,
	})

	// activities
	if client.Cache.Status != "" {
		type PresenceUpdateData struct {
			Status     string             `json:"status"`
			Since      int                `json:"since"`
			Activities []PresenceActivity `json:"activities"`
			Afk        bool               `json:"afk"`
		}

		client.SendWSMessage(OpcodePresenceUpdate, PresenceUpdateData{
			Status:     client.Cache.Status,
			Since:      0,
			Activities: client.Cache.Activities,
			Afk:        false,
		})
	}

	// reason i did this:
	// for big communities, you don't get all messages incoming unless you are listening to them
	// so.. i made this get the guild with most members, which would theoretically be a big community
	// and listen to it. this does not affect getting the other messages incoming!
	// otherwise we wouldn't get MESSAGE_CREATE coming from this big community
	// (i really hope there's a way to do this better and listen to all big communities, but idk any rn)
	if client.Cache.GuildWithMostMembersID != "" {
		type GuildSubscriptionData struct {
			GuildID    string `json:"guild_id"`
			Typing     bool   `json:"typing"`
			Threads    bool   `json:"threads"`
			Activities bool   `json:"activities"`
			// Members           []string           `json:"members"`
			// Channels          map[string][][]int `json:"channels"`
			// ThreadMemberLists []string           `json:"thread_member_lists"`
		}

		client.SendWSMessage(Opcode(14), GuildSubscriptionData{
			GuildID:    client.Cache.GuildWithMostMembersID,
			Typing:     true,
			Threads:    true, // false
			Activities: true, // false
			// Members:           []string{},
			// Channels:          make(map[string][][]int),
			// ThreadMemberLists: []string{},
		})
	}
}

func (client *Client) SendWSMessage(op Opcode, data interface{}) error {
	client.Lock()
	defer client.Unlock()

	if client.conn == nil {
		return ErrWSNotFound
	}

	// data, err := json.Marshal(wsMSG)
	// if err != nil {
	// 	return err
	// }

	// err = client.conn.WriteMessage(websocket.TextMessage, data)

	jsData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// fmt.Println("sending ws message, OP:", op, "DATA:", string(jsData))

	return client.conn.WriteJSON(WSMessage{
		Op: op,
		D:  jsData,
	})
}

func (client *Client) onMessage(messageType int, message []byte) (*WSMessage, error) {
	var err error
	var reader io.Reader
	reader = bytes.NewBuffer(message)

	// If this is a compressed message, uncompress it.
	// this is not needed since we don't pass the compress arg in the gateway url
	if messageType == websocket.BinaryMessage {

		z, err2 := zlib.NewReader(reader)
		if err2 != nil {
			// fmt.Println("[onMessage] error uncompressing websocket message,", err2)
			return nil, err2
		}

		defer func() {
			// err3 := z.Close()
			// if err3 != nil {
			// 	fmt.Println("[onMessage] error closing zlib,", err3)
			// }
			_ = z.Close()
		}()

		reader = z
	}

	// Decode the event into an Event struct.
	var e *WSMessage
	decoder := json.NewDecoder(reader)
	if err = decoder.Decode(&e); err != nil {
		// fmt.Println("[onMessage] error decoding websocket message,", err)
		return e, err
	}

	//fmt.Println("got message, op:", e.Op, "d:", string(e.D))
	//fmt.Println("got message, op:", e.Op)

	return e, nil
}

func (client *Client) heartbeat() {
	heartbeatTicker := time.NewTicker(client.heartbeatInterval)
	defer heartbeatTicker.Stop()

	// defer fmt.Println("exiting heartbeat goroutine...")

	for {
		select {
		case <-client.heartbeatChan:
			return

		case <-heartbeatTicker.C:
			client.sendHeartbeat()
		}
	}
}

func (client *Client) sendHeartbeat() {
	client.RLock()
	lastSequenceReceived := client.LastSequenceReceived
	client.RUnlock()

	if err := client.SendWSMessage(OpcodeHeartbeat, HeartBeatMessage(lastSequenceReceived)); err != nil {
		if err == ErrWSNotFound || errors.Is(err, syscall.EPIPE) {
			return
		}

		//client.Close()
		client.CloseWithCode(websocket.CloseServiceRestart)
		go client.reconnect()

		return
	}

	// client.lastHeartbeatSent = time.Now().UTC()
}

func (client *Client) listenForMessages(conn *websocket.Conn) {
loop:
	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			client.RLock()
			sameConn := client.conn == conn
			client.RUnlock()

			if !sameConn {
				return
			}

			var reconnect = true
			if closeError, ok := err.(*websocket.CloseError); ok {
				closeCode := CloseEventCodeByCode(closeError.Code)
				reconnect = closeCode.Reconnect

				if closeCode == CloseEventCodeInvalidSeq {
					client.Lock()
					client.LastSequenceReceived = 0
					client.SessionID = ""
					client.gateawyURL = ""
					client.Unlock()
				}
			}

			//client.Close()
			client.CloseWithCode(websocket.CloseServiceRestart)
			if reconnect {
				go client.reconnect()
			}

			// fmt.Println("[listenForMessages] There has been an error reading incomming message", err)
			return
		}

		select {
		case <-client.messagesChan:
			return
		default:
		}

		receivedMsg, err := client.onMessage(msgType, msg)
		if err != nil {
			// fmt.Println("[listenForMessages] [onMessage failed], error:", err)
			continue
		}

		switch receivedMsg.Op {
		case OpcodeDispatch:
			switch receivedMsg.T {
			case EventTypeMessageCreate:
				if client.OnMessageCreate != nil {
					var msg DiscordMessage
					if err = json.Unmarshal(receivedMsg.D, &msg); err == nil {
						client.OnMessageCreate(&msg)
					} // if the error is not nil, then we're fucked
				}

			case EventTypeReady:
				var ready ReadyMessage
				if err = json.Unmarshal(receivedMsg.D, &ready); err == nil {
					client.Lock()
					client.SessionID = ready.SessionID
					client.gateawyURL = ready.ResumeGatewayURL
					client.Unlock()

					client.Cache.User = ready.User
					for _, guild := range ready.Guilds {
						client.Cache.SetGuildName(guild.ID, guild.Properties.Name)
						// client.Cache.SetGuildName(guild.ID, guild.Name)
					}

					client.Cache.Status = global.GetConfigAltsStatus()
					if !global.Config.Alts.ForceStatus {
						for _, session := range ready.Sessions {
							if session.SessionID == "all" || session.Active {
								client.Cache.Status = session.Status
								client.Cache.Activities = session.Activities
								break
							}
						}
					}

					var guildWithMostMembers *DiscordGuild = nil
					for i := range ready.Guilds {
						guild := &ready.Guilds[i]
						if guildWithMostMembers == nil || guild.MemberCount > guildWithMostMembers.MemberCount {
							guildWithMostMembers = guild
						}
					}

					client.Cache.GuildWithMostMembersID = ""
					if guildWithMostMembers != nil {
						client.Cache.GuildWithMostMembersID = guildWithMostMembers.ID
					}

					client.sendClientData()

					if client.OnReady != nil {
						client.OnReady(&ready)
					}
				}

			case EventTypeGuildCreate:
				var guild GuildCreate
				if err = json.Unmarshal(receivedMsg.D, &guild); err == nil {
					client.Cache.SetGuildName(guild.ID, guild.Properties.Name)
					// client.Cache.SetGuildName(guild.ID, guild.Name)
				}

			case EventTypeGuildUpdate:
				var guild GuildUpdate
				if err = json.Unmarshal(receivedMsg.D, &guild); err == nil {
					client.Cache.SetGuildName(guild.ID, guild.Name)
				}
			case EventTypeGuildDelete:
				var guild GuildDelete
				if err = json.Unmarshal(receivedMsg.D, &guild); err == nil {
					client.Cache.RemoveGuild(guild.ID)
				}
			}

			// set last sequence received
			// client.Lock()
			client.LastSequenceReceived = receivedMsg.S
			// client.Unlock()
		case OpcodeHeartbeat:
			client.sendHeartbeat()

		case OpcodeReconnect:
			client.CloseWithCode(websocket.CloseServiceRestart)
			go client.reconnect()
			break loop

		case OpcodeInvalidSession:
			var canResume bool
			_ = json.Unmarshal(receivedMsg.D, &canResume)

			code := websocket.CloseNormalClosure
			if canResume {
				code = websocket.CloseServiceRestart
			} else {
				// clear resume info
				client.Lock()
				client.LastSequenceReceived = 0
				client.SessionID = ""
				client.gateawyURL = ""
				client.Unlock()
			}

			client.CloseWithCode(code)
			go client.reconnect()
			break loop

		case OpcodeHello:
			hello := HelloMessage{}
			_ = json.Unmarshal(receivedMsg.D, &hello)

			client.Lock()
			client.heartbeatInterval = hello.HeartbeatInterval * time.Millisecond
			// client.lastHeartbeatReceived = time.Now().UTC()
			client.Unlock()

			go client.heartbeat()

			client.RLock()
			sessionID := client.SessionID
			lastSequenceReceived := client.LastSequenceReceived
			client.RUnlock()

			if sessionID == "" && lastSequenceReceived == 0 {
				_ = client.identifyNew()
			} else {
				_ = client.resume()
				client.sendClientData()
			}

			// case OpcodeHeartbeatACK:
			// 	client.Lock()
			// 	client.lastHeartbeatReceived = time.Now().UTC()
			// 	client.Unlock()

		}

	}
}
