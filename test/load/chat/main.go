// Command chat drives the chat service the way its users do: each virtual
// user takes a session, joins a matching queue, connects to the room it is
// matched into, sends a message at a fixed interval and leaves. It reports how
// many users got through each step and how long a message took to reach the
// other participants of its room.
//
// Every virtual user runs in this one process, so a message is timed from the
// moment it was written to the moment another user read it, on the same clock.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

type config struct {
	base          *url.URL
	origin        string
	users         int
	rampRate      float64
	stay          time.Duration
	interval      time.Duration
	bodyLen       int
	roomType      int
	topics        []int
	matchTimeout  time.Duration
	reportEvery   time.Duration
	handshakeWait time.Duration
}

func main() {
	cfg, err := parseFlags()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err = run(ctx, cfg)
	stop()
	if err != nil {
		log.Fatal(err)
	}
}

func parseFlags() (config, error) {
	var (
		cfg    config
		base   string
		topics string
	)
	flag.StringVar(&base, "base", "http://localhost:8080", "API の URL。WebSocket の接続先もここから作る")
	flag.StringVar(&cfg.origin, "origin", "", "WebSocket のハンドシェイクに付ける Origin。空なら -base と同じ")
	flag.IntVar(&cfg.users, "users", 100, "仮想ユーザーの数")
	flag.Float64Var(&cfg.rampRate, "ramp", 20, "1 秒あたりに開始する仮想ユーザーの数")
	flag.DurationVar(&cfg.stay, "stay", 5*time.Minute, "接続してから退出するまでの時間")
	flag.DurationVar(&cfg.interval, "interval", 10*time.Second, "1 人がメッセージを送る間隔")
	flag.IntVar(&cfg.bodyLen, "body-len", 40, "メッセージ本文のおおよその文字数")
	flag.IntVar(&cfg.roomType, "room-type", 2, "入る待機キューのルーム種別（2 か 3）")
	flag.StringVar(&topics, "topics", "", "使うトピックの ID（カンマ区切り）。空なら GET /api/topics の全件")
	flag.DurationVar(&cfg.matchTimeout, "match-timeout", 2*time.Minute, "マッチングを待つ上限")
	flag.DurationVar(&cfg.reportEvery, "report-every", 10*time.Second, "途中経過を出す間隔")
	flag.DurationVar(&cfg.handshakeWait, "handshake-timeout", 10*time.Second, "WebSocket のハンドシェイクを待つ上限")
	flag.Parse()

	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return cfg, fmt.Errorf("-base は http(s)://host の形で指定する: %q", base)
	}
	cfg.base = u
	if cfg.origin == "" {
		cfg.origin = u.Scheme + "://" + u.Host
	}
	if cfg.users < 1 || cfg.rampRate <= 0 || cfg.interval <= 0 || cfg.stay <= 0 {
		return cfg, errors.New("-users・-ramp・-interval・-stay は正の値で指定する")
	}
	if cfg.roomType != 2 && cfg.roomType != 3 {
		return cfg, errors.New("-room-type は 2 か 3 で指定する")
	}
	for _, s := range strings.Split(topics, ",") {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		id, err := strconv.Atoi(s)
		if err != nil {
			return cfg, fmt.Errorf("-topics に数字でない ID がある: %q", s)
		}
		cfg.topics = append(cfg.topics, id)
	}
	return cfg, nil
}

func run(ctx context.Context, cfg config) error {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        cfg.users,
			MaxIdleConnsPerHost: cfg.users,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	if len(cfg.topics) == 0 {
		topics, err := listTopics(ctx, httpClient, cfg.base)
		if err != nil {
			return err
		}
		cfg.topics = topics
	}

	st := newStats()
	reg := newSentRegistry()
	log.Printf("start: users=%d ramp=%.1f/s stay=%s interval=%s room_type=%d topics=%v",
		cfg.users, cfg.rampRate, cfg.stay, cfg.interval, cfg.roomType, cfg.topics)

	reportCtx, stopReport := context.WithCancel(ctx)
	defer stopReport()
	go func() {
		t := time.NewTicker(cfg.reportEvery)
		defer t.Stop()
		for {
			select {
			case <-reportCtx.Done():
				return
			case <-t.C:
				log.Print(st.progress())
			}
		}
	}()

	var wg sync.WaitGroup
	gap := time.Duration(float64(time.Second) / cfg.rampRate)
	started := time.Now()
	for i := range cfg.users {
		if ctx.Err() != nil {
			break
		}
		// Users who enter one after another share a queue, so that they are
		// matched with each other rather than left waiting for a partner.
		topic := cfg.topics[(i/cfg.roomType)%len(cfg.topics)]
		u := &user{
			cfg:   cfg,
			http:  httpClient,
			stats: st,
			sent:  reg,
			topic: topic,
		}
		wg.Go(func() { u.run(ctx) })

		// Pace from the start time so that slow iterations do not drift.
		if wait := time.Until(started.Add(time.Duration(i+1) * gap)); wait > 0 {
			select {
			case <-ctx.Done():
			case <-time.After(wait):
			}
		}
	}
	wg.Wait()
	stopReport()

	fmt.Println(st.summary())
	return nil
}

func listTopics(ctx context.Context, c *http.Client, base *url.URL) ([]int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.JoinPath("/api/topics").String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list topics: status %d", resp.StatusCode)
	}

	var body struct {
		Topics []struct {
			ID int `json:"id"`
		} `json:"topics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode topics: %w", err)
	}
	ids := make([]int, 0, len(body.Topics))
	for _, t := range body.Topics {
		ids = append(ids, t.ID)
	}
	if len(ids) == 0 {
		return nil, errors.New("no topics to join")
	}
	return ids, nil
}

// user is one virtual user. It holds the cookies the server gave it and
// sends them back by hand, so that cookies marked Secure reach a server
// served over plain HTTP as well.
type user struct {
	cfg     config
	http    *http.Client
	stats   *stats
	sent    *sentRegistry
	topic   int
	cookies map[string]string
}

func (u *user) run(ctx context.Context) {
	u.cookies = map[string]string{}
	defer u.leave()

	if err := u.issueSession(ctx); err != nil {
		u.stats.fail("session", err)
		return
	}
	u.stats.inc("session_ok")

	roomID, err := u.match(ctx)
	if err != nil {
		u.stats.fail("matching", err)
		return
	}
	u.stats.inc("matched")

	u.chat(ctx, roomID)
}

// statusError is a response the scenario did not expect. Its status code is
// what the summary groups failures by.
type statusError struct {
	step   string
	status int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("%s: status %d", e.step, e.status)
}

func (u *user) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.cfg.base.JoinPath(path).String(), reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if h := u.cookieHeader(); h != "" {
		req.Header.Set("Cookie", h)
	}

	resp, err := u.http.Do(req)
	if err != nil {
		return nil, err
	}
	for _, c := range resp.Cookies() {
		if c.MaxAge < 0 {
			delete(u.cookies, c.Name)
			continue
		}
		u.cookies[c.Name] = c.Value
	}
	return resp, nil
}

func (u *user) cookieHeader() string {
	parts := make([]string, 0, len(u.cookies))
	for name, value := range u.cookies {
		parts = append(parts, name+"="+value)
	}
	return strings.Join(parts, "; ")
}

func (u *user) issueSession(ctx context.Context) error {
	resp, err := u.do(ctx, http.MethodPost, "/api/session", nil)
	if err != nil {
		return err
	}
	drain(resp)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return &statusError{step: "session", status: resp.StatusCode}
	}
	return nil
}

type matchingState struct {
	State        string `json:"state"`
	Conversation *struct {
		ID string `json:"id"`
	} `json:"conversation"`
}

// match joins the queue and waits until a room is formed. A join refused for
// its rate is tried again after the time the server asks for.
func (u *user) match(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, u.cfg.matchTimeout)
	defer cancel()

	join := map[string]int{"topic_id": u.topic, "room_type": u.cfg.roomType}
	for {
		resp, err := u.do(ctx, http.MethodPost, "/api/matching", join)
		if err != nil {
			return "", err
		}
		switch resp.StatusCode {
		case http.StatusOK, http.StatusAccepted:
			state, err := decodeState(resp)
			if err != nil {
				return "", err
			}
			if id, ok := state.roomID(); ok {
				return id, nil
			}
			return u.poll(ctx)
		case http.StatusTooManyRequests:
			u.stats.inc("matching_rate_limited")
			wait := retryAfter(resp)
			drain(resp)
			if err := sleep(ctx, wait); err != nil {
				return "", err
			}
		default:
			drain(resp)
			return "", &statusError{step: "matching", status: resp.StatusCode}
		}
	}
}

func (u *user) poll(ctx context.Context) (string, error) {
	for {
		if err := sleep(ctx, time.Second); err != nil {
			return "", err
		}
		resp, err := u.do(ctx, http.MethodGet, "/api/matching", nil)
		if err != nil {
			return "", err
		}
		if resp.StatusCode != http.StatusOK {
			drain(resp)
			return "", &statusError{step: "matching_poll", status: resp.StatusCode}
		}
		state, err := decodeState(resp)
		if err != nil {
			return "", err
		}
		if id, ok := state.roomID(); ok {
			return id, nil
		}
	}
}

func decodeState(resp *http.Response) (matchingState, error) {
	defer drain(resp)
	var s matchingState
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return s, fmt.Errorf("decode matching state: %w", err)
	}
	return s, nil
}

func (s matchingState) roomID() (string, bool) {
	if s.State != "matched" || s.Conversation == nil {
		return "", false
	}
	return s.Conversation.ID, true
}

// serverEvent holds the fields of a server event this command reads.
type serverEvent struct {
	Type        string `json:"type"`
	Participant int    `json:"participant"`
	Body        string `json:"body"`
	Reason      string `json:"reason"`
	Code        string `json:"code"`
}

func (u *user) chat(ctx context.Context, roomID string) {
	wsURL := *u.cfg.base
	wsURL.Scheme = map[string]string{"http": "ws", "https": "wss"}[wsURL.Scheme]
	wsURL.Path = "/ws/rooms/" + roomID

	header := http.Header{}
	header.Set("Origin", u.cfg.origin)
	header.Set("Cookie", u.cookieHeader())

	dialer := websocket.Dialer{HandshakeTimeout: u.cfg.handshakeWait}
	dialStart := time.Now()
	conn, resp, err := dialer.DialContext(ctx, wsURL.String(), header)
	if resp != nil {
		drain(resp)
	}
	if err != nil {
		if resp != nil {
			err = &statusError{step: "websocket", status: resp.StatusCode}
		}
		u.stats.fail("websocket", err)
		return
	}
	u.stats.observe("handshake", time.Since(dialStart))
	u.stats.connected(1)
	defer u.stats.connected(-1)
	defer conn.Close()

	// The read loop answers the server's pings: gorilla/websocket writes the
	// pong from inside ReadMessage.
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		u.read(conn)
	}()

	u.write(ctx, conn, ended)

	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second))
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
	}
}

func (u *user) read(conn *websocket.Conn) {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) &&
				!errors.Is(err, websocket.ErrCloseSent) {
				u.stats.inc("read_error")
			}
			return
		}
		received := time.Now()

		var ev serverEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			u.stats.inc("event_undecodable")
			continue
		}
		u.stats.inc("event_" + ev.Type)
		switch ev.Type {
		case "message":
			if sentAt, own, ok := u.sent.lookup(ev.Body, u); ok && !own {
				u.stats.observe("delivery", received.Sub(sentAt))
			}
		case "error":
			u.stats.inc("error_" + ev.Code)
		case "ended":
			u.stats.inc("ended_" + ev.Reason)
		}
	}
}

// write sends a message every interval until the user's stay is over, the
// run is stopped, or the server ends the conversation. The first message is
// delayed by a random part of the interval so that users do not send in step.
func (u *user) write(ctx context.Context, conn *websocket.Conn, ended <-chan struct{}) {
	stayOver := time.After(u.cfg.stay)
	next := time.After(rand.N(u.cfg.interval))
	for {
		select {
		case <-ctx.Done():
			return
		case <-ended:
			return
		case <-stayOver:
			return
		case <-next:
			next = time.After(u.cfg.interval)
		}

		body := u.sent.record(u, u.cfg.bodyLen)
		frame, _ := json.Marshal(map[string]string{"type": "message", "body": body})
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
			u.stats.inc("write_error")
			return
		}
		u.stats.inc("sent")
	}
}

// leave gives back what the user holds on the server, as a browser does when
// its user leaves: its place in the queue and its session.
func (u *user) leave() {
	if len(u.cookies) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, path := range []string{"/api/matching", "/api/session"} {
		resp, err := u.do(ctx, http.MethodDelete, path, nil)
		if err != nil {
			u.stats.inc("leave_error")
			continue
		}
		drain(resp)
	}
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func retryAfter(resp *http.Response) time.Duration {
	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
		return time.Duration(s) * time.Second
	}
	return 10 * time.Second
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
