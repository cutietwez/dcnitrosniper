package request

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"sniper/global"
	"strconv"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
)

var (
	claimer ClaimRequester

	userAgent string

	DiscordHost     string = "canary.discord.com" // "ptb.discord.com" // "discord.com"
	APIVersion      string = "9"
	FullDiscordHost string = "https://canary.discord.com/api/v9"

	rawTlsConfig = &tls.Config{
		ClientSessionCache:     tls.NewLRUClientSessionCache(1000),
		SessionTicketsDisabled: false,
		MinVersion:             tls.VersionTLS13,
		MaxVersion:             tls.VersionTLS13,
		InsecureSkipVerify:     true,
	}
)

type ClaimRequester interface {
	// Just initialize
	Init(Token string)

	// Called when the token changes
	OnClaimTokenChange(Token string)

	// Return: {statusCode, responseBody, endTime, error}
	ClaimCode(code string) (int, string, time.Time, error)

	// Called to dial discord
	DialDiscord()
}

// called in main.go
func Init(UserAgent, Token string) {
	userAgent = UserAgent

	switch global.Config.Sniper.SnipeType {
	case 0:
		claimer = &fasthttpClaimRequester{}
	case 1:
		claimer = &nethttpClaimRequester{}
	case 2:
		claimer = &dialClaimRequester{}
	default:
		claimer = &fasthttpClaimRequester{}
	}

	// get APIVersion
	if num, err := strconv.Atoi(strings.TrimSpace(global.Config.Discord.APIVersion)); err == nil && num >= 6 && num <= 10 {
		APIVersion = strconv.Itoa(num)
	} else {
		APIVersion = "9"
	}

	// get DiscordHost
	if global.Config.Discord.HostSelection == nil {
		DiscordHost = "canary.discord.com"
	} else {
		switch *global.Config.Discord.HostSelection {
		case 0:
			DiscordHost = "discord.com"
		case 1:
			DiscordHost = "discordapp.com"
		case 2:
			DiscordHost = "ptb.discord.com"
		case 3:
			DiscordHost = "ptb.discordapp.com"
		case 4:
			DiscordHost = "canary.discord.com"
		case 5:
			DiscordHost = "canary.discordapp.com"
		case 6:
			DiscordHost = "canary-api.discordapp.com"
		default:
			DiscordHost = "canary.discord.com"
		}
	}

	// set full discord host, which we will use for sniping. this CAN not include api version
	FullDiscordHost = "https://" + DiscordHost + "/api"
	if global.Config.Discord.APIVersion != "" {
		FullDiscordHost = FullDiscordHost + "/v" + APIVersion
	}

	// finally initialize it
	claimer.Init(Token)

	// create the DialDiscord goroutine
	go func() {
		for !global.ShouldKill {
			claimer.DialDiscord()
			time.Sleep(time.Second * 10)
		}
	}()
}

// called in main.go
func OnClaimTokenChange(Token string) {
	claimer.OnClaimTokenChange(Token)
}

// called by snipers
// Return: {statusCode, responseBody, endTime, error}
func ClaimCode(code string) (int, string, time.Time, error) {
	return claimer.ClaimCode(code)
}

type fasthttpClaimRequester struct {
	fasthttpClient  *fasthttp.Client
	fasthttpReq     *fasthttp.Request
	fasthttpDialReq *fasthttp.Request
}

func (c *fasthttpClaimRequester) Init(Token string) {
	c.fasthttpClient = &fasthttp.Client{
		Dial: func(addr string) (net.Conn, error) {
			return fasthttp.DialTimeout(addr, 10*time.Second)
		},
		MaxConnsPerHost:     10,
		MaxIdleConnDuration: 60 * time.Second,
		TLSConfig:           rawTlsConfig,
		/*ConfigureClient: func(hc *fasthttp.HostClient) error {
			hc.Addr = "discord.com:443"
			hc.MaxConns = 100
			hc.MaxIdleConnDuration = 60 * time.Second
			return nil
		},*/
	}

	c.fasthttpReq = fasthttp.AcquireRequest()
	c.fasthttpReq.SetBodyString("{}")
	c.fasthttpReq.Header.SetMethod(fasthttp.MethodPost)
	c.fasthttpReq.Header.SetContentType("application/json")
	c.fasthttpReq.Header.SetUserAgent(userAgent)
	c.fasthttpReq.Header.Set("Connection", "keep-alive")
	c.fasthttpReq.Header.Set("Authorization", Token)
	c.fasthttpReq.Header.Set("X-Discord-Locale", "en-US")
	c.fasthttpReq.SetRequestURI(FullDiscordHost + "/entitlements/gift-codes/" + "xxx" + "/redeem")

	c.fasthttpDialReq = fasthttp.AcquireRequest()
	c.fasthttpDialReq.Header.SetMethod(fasthttp.MethodGet)
	c.fasthttpDialReq.Header.SetContentType("application/json")
	c.fasthttpDialReq.Header.SetUserAgent(userAgent)
	c.fasthttpDialReq.Header.Set("Connection", "keep-alive")
	c.fasthttpDialReq.Header.Set("X-Discord-Locale", "en-US")
	c.fasthttpDialReq.SetRequestURI(FullDiscordHost + "/entitlements/gift-codes/" + "xxx" + "/redeem")
}

func (c *fasthttpClaimRequester) OnClaimTokenChange(Token string) {
	c.fasthttpReq.Header.Set("Authorization", Token)
}

// Return: {statusCode, responseBody, endTime, error}
func (c *fasthttpClaimRequester) ClaimCode(code string) (int, string, time.Time, error) {
	res := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(res)

	c.fasthttpReq.SetRequestURI(FullDiscordHost + "/entitlements/gift-codes/" + code + "/redeem")

	err := c.fasthttpClient.Do(c.fasthttpReq, res)
	endTime := time.Now()

	if err != nil {
		return 0, "", endTime, err
	}

	return res.StatusCode(), string(res.Body()), endTime, nil
}

func (c *fasthttpClaimRequester) DialDiscord() {
	var resp *fasthttp.Response = fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	_ = c.fasthttpClient.Do(c.fasthttpDialReq, resp)
}

type nethttpClaimRequester struct {
	httpClient         *http.Client
	claimHeaders       http.Header
	dialRequestHeaders http.Header
}

func (c *nethttpClaimRequester) Init(Token string) {
	c.httpClient = &http.Client{
		Transport: &http.Transport{
			//TLSClientConfig:     &tls.Config{CipherSuites: []uint16{0x1301}, InsecureSkipVerify: true, PreferServerCipherSuites: true, MinVersion: 0x0304},
			TLSClientConfig:     rawTlsConfig,
			DisableKeepAlives:   false,
			MaxIdleConnsPerHost: 1000,
			ForceAttemptHTTP2:   true,
			DisableCompression:  false,
			IdleConnTimeout:     0,
			MaxIdleConns:        0,
			MaxConnsPerHost:     0,
			//TLSNextProto:        make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		},
		Timeout: 0,
	}

	c.claimHeaders = http.Header{
		"Content-Type":     {"application/json"},
		"Authorization":    {Token},
		"User-Agent":       {userAgent},
		"Connection":       {"keep-alive"},
		"X-Discord-Locale": {"en-US"},
	}

	c.dialRequestHeaders = http.Header{
		"Content-Type":     {"application/json"},
		"User-Agent":       {userAgent},
		"Connection":       {"keep-alive"},
		"X-Discord-Locale": {"en-US"},
	}
}

func (c *nethttpClaimRequester) OnClaimTokenChange(Token string) {
	c.claimHeaders = http.Header{
		"Content-Type":     {"application/json"},
		"Authorization":    {Token},
		"User-Agent":       {userAgent},
		"Connection":       {"keep-alive"},
		"X-Discord-Locale": {"en-US"},
	}

	c.dialRequestHeaders = http.Header{
		"Content-Type":     {"application/json"},
		"User-Agent":       {userAgent},
		"X-Discord-Locale": {"en-US"},
	}
}

// Return: {statusCode, responseBody, endTime, error}
func (c *nethttpClaimRequester) ClaimCode(code string) (int, string, time.Time, error) {
	// todo: we could improve this A LOT by preparing the "http.NewRequest" and the body buffer
	request, requestErr := http.NewRequest("POST", FullDiscordHost+"/entitlements/gift-codes/"+code+"/redeem", bytes.NewReader([]byte("{}")))
	if requestErr != nil {
		return 0, "", time.Now(), requestErr
	}

	request.Header = c.claimHeaders

	response, responseErr := c.httpClient.Do(request)
	endTime := time.Now()

	if responseErr != nil {
		return 0, "", endTime, responseErr
	}

	defer response.Body.Close()

	bodyBytes, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(bodyBytes), endTime, nil
}

func (c *nethttpClaimRequester) DialDiscord() {
	request, requestErr := http.NewRequest("GET", FullDiscordHost+"/entitlements/gift-codes/"+"xxx"+"/redeem", nil)
	if requestErr != nil {
		return
	}

	request.Header = c.dialRequestHeaders

	response, responseErr := c.httpClient.Do(request)
	if responseErr != nil {
		return
	}

	response.Body.Close()
}

type dialClaimRequester struct {
	claimToken string
}

func (c *dialClaimRequester) Init(Token string) {
	c.claimToken = Token
}

func (c *dialClaimRequester) OnClaimTokenChange(Token string) {
	c.claimToken = Token
}

// Return: {statusCode, responseBody, endTime, error}
func (c *dialClaimRequester) ClaimCode(code string) (int, string, time.Time, error) {
	discordConn, err := tls.Dial("tcp", DiscordHost+":443", rawTlsConfig)
	if err != nil {
		return 0, "", time.Now(), err
	}

	discordConn.Write([]byte("POST /api/v" + APIVersion + "/entitlements/gift-codes/" + code + "/redeem HTTP/1.1\r\nHost: " + DiscordHost + "\r\nAuthorization: " + c.claimToken + "\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n{}"))

	response, err := http.ReadResponse(bufio.NewReader(discordConn), nil)

	endTime := time.Now() // tbh, i think i should put it up, before reading response, but ok
	if err != nil {
		return 0, "", endTime, err
	}

	bodyBytes, _ := io.ReadAll(response.Body)
	response.Body.Close()

	discordConn.Close()

	return response.StatusCode, string(bodyBytes), endTime, nil
}

func (c *dialClaimRequester) DialDiscord() {
	// empty
}
