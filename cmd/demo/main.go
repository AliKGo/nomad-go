package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// ── Config ────────────────────────────────────────────────────────────────────

const (
	base     = "http://192.168.223.128"
	wsBase   = "ws://192.168.223.128"
	grafana  = "http://192.168.223.128:3000"
	password = "Astana2026@"
)

type loc struct {
	lat, lon float64
	addr     string
}

var locations = []loc{
	{43.2220, 76.8512, "Достык 136, Алматы"},
	{43.2389, 76.8897, "Абая 42, Алматы"},
	{43.2567, 76.9435, "MEGA Center"},
	{43.2775, 76.9225, "Медеу"},
	{43.2052, 76.6984, "Аэропорт Алматы"},
	{43.2642, 76.9268, "Горный Гигант"},
	{43.2536, 76.9126, "ЦУМ Алматы"},
	{43.2311, 76.9156, "Арбат, Алматы"},
	{43.2701, 76.9312, "Парк Горького"},
	{43.2461, 76.9001, "Рамстор, Алматы"},
	{43.2199, 76.8731, "Зелёный Базар"},
	{43.2888, 76.9601, "Шымбулак"},
}

var (
	rideTypes = []string{"ECONOMY", "ECONOMY", "ECONOMY", "PREMIUM", "XL"}
	makes     = []string{"Toyota", "Hyundai", "Kia", "BMW", "Mercedes", "Chevrolet", "Lada"}
	models    = []string{"Camry", "Sonata", "Rio", "5 Series", "E-Class", "Cobalt", "Vesta"}
	colors    = []string{"White", "Black", "Silver", "Gray", "Blue", "Red"}

	// плохие запросы разного вида для генерации 4xx
	badPasswords = []string{"wrongpass", "123456", "", "admin", "password", "qwerty"}
	fakeEmails   = []string{
		"notexist@demo.com",
		"fake_user@test.com",
		"ghost@nomad.kz",
	}
)

// ── Counters ──────────────────────────────────────────────────────────────────

var (
	totalRides   atomic.Int64
	totalReqs    atomic.Int64
	totalErrors  atomic.Int64
	iteration    atomic.Int64
)

// ── Terminal colours ──────────────────────────────────────────────────────────

var mu sync.Mutex

func ts() string { return time.Now().Format("15:04:05") }

func logOk(msg string)  { mu.Lock(); fmt.Printf("\033[32m[%s] ✓  %s\033[0m\n", ts(), msg); mu.Unlock() }
func logWarn(msg string) { mu.Lock(); fmt.Printf("\033[33m[%s] ⚠  %s\033[0m\n", ts(), msg); mu.Unlock() }
func logInfo(msg string) { mu.Lock(); fmt.Printf("\033[36m[%s]    %s\033[0m\n", ts(), msg); mu.Unlock() }
func section(msg string) {
	bar := "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	mu.Lock()
	fmt.Printf("\n\033[35m%s\n  %s\n%s\033[0m\n", bar, msg, bar)
	mu.Unlock()
}

// ── HTTP client ───────────────────────────────────────────────────────────────

var client = &http.Client{Timeout: 8 * time.Second}

func req(method, path string, body any, token string) map[string]any {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}

	httpReq, _ := http.NewRequest(method, base+path, r)
	httpReq.Header.Set("Content-Type", "application/json")
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		logWarn(fmt.Sprintf("%-6s %s → %v", method, path, err))
		totalErrors.Add(1)
		return nil
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	totalReqs.Add(1)

	label := fmt.Sprintf("%-6s %-42s %d", method, path, resp.StatusCode)
	if resp.StatusCode < 400 {
		logOk(label)
	} else {
		logWarn(label)
		totalErrors.Add(1)
	}

	var result map[string]any
	json.Unmarshal(raw, &result)
	return result
}

func post(path string, body any, token string) map[string]any { return req("POST", path, body, token) }
func get(path, token string) map[string]any                   { return req("GET", path, nil, token) }

func str(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

func randLoc() loc             { return locations[rand.Intn(len(locations))] }
func randOf(s []string) string { return s[rand.Intn(len(s))] }
func randF(min, max float64) float64 {
	return min + rand.Float64()*(max-min)
}

// ── Auth ──────────────────────────────────────────────────────────────────────

func register(name, phone, email string) string {
	r := post("/auth/register", map[string]any{
		"name": name, "phone": phone, "email": email, "password": password,
	}, "")
	return str(r, "id")
}

type creds struct{ access, refresh string }

func loginAs(email string) creds {
	r := post("/auth/login", map[string]any{"email": email, "password": password}, "")
	return creds{str(r, "access_token"), str(r, "refresh_token")}
}

func me(token string) { get("/auth/me", token) }

func refreshCreds(ref string) creds {
	r := post("/auth/refresh", map[string]any{"refresh_token": ref}, "")
	return creds{str(r, "access_token"), str(r, "refresh_token")}
}

// ── Driver ────────────────────────────────────────────────────────────────────

func registerDriver(driverID string) {
	post("/driver/drivers", map[string]any{
		"id":             driverID,
		"name":           "Driver " + driverID[:6],
		"license_number": fmt.Sprintf("KZ%d", rand.Intn(900000)+100000),
		"vehicle": map[string]any{
			"make":  randOf(makes),
			"model": randOf(models),
			"color": randOf(colors),
			"plate": fmt.Sprintf("%c%d%c%c", 'A'+rand.Intn(6), rand.Intn(900)+100, 'A'+rand.Intn(6), 'A'+rand.Intn(6)),
			"year":  rand.Intn(7) + 2017,
			"type":  randOf([]string{"ECONOMY", "ECONOMY", "PREMIUM"}),
		},
	}, "")
}

func driverOnline(driverID, token string) {
	l := randLoc()
	post("/driver/drivers/"+driverID+"/online", map[string]any{
		"latitude": l.lat, "longitude": l.lon,
	}, token)
}

func driverOffline(driverID, token string) {
	post("/driver/drivers/"+driverID+"/offline", map[string]any{}, token)
}

func driverLocation(driverID, token string) {
	l := randLoc()
	post("/driver/drivers/"+driverID+"/location", map[string]any{
		"latitude":        l.lat + randF(-0.005, 0.005),
		"longitude":       l.lon + randF(-0.005, 0.005),
		"accuracy_meters": randF(2, 15),
		"speed_kmh":       randF(0, 90),
		"heading_degrees": randF(0, 360),
	}, token)
}

// ── Ride ──────────────────────────────────────────────────────────────────────

func createRide(passengerID, token string) string {
	pickup := randLoc()
	dest := randLoc()
	r := post("/ride/rides", map[string]any{
		"passenger_id":          passengerID,
		"pickup_latitude":        pickup.lat,
		"pickup_longitude":       pickup.lon,
		"pickup_address":         pickup.addr,
		"destination_latitude":   dest.lat,
		"destination_longitude":  dest.lon,
		"destination_address":    dest.addr,
		"ride_type":              randOf(rideTypes),
	}, token)
	return str(r, "ride_id")
}

func cancelRide(rideID, token string) {
	reasons := []string{"demo", "changed mind", "too long wait", "wrong location", "found alternative"}
	post("/ride/rides/"+rideID+"/cancel", map[string]any{"reason": randOf(reasons)}, token)
}

// ── Scenarios (горутины) ──────────────────────────────────────────────────────

// passengerWorker — создаёт поездки и изредка их отменяет
func passengerWorker(stop <-chan struct{}, id, token string, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			// GET /auth/me несколько раз
			n := rand.Intn(3) + 1
			for i := 0; i < n; i++ {
				me(token)
			}
			// Создать поездку
			rideID := createRide(id, token)
			if rideID != "" {
				totalRides.Add(1)
				// 70% — отмена через случайную задержку, 30% — оставить висеть
				if rand.Intn(10) < 7 {
					time.Sleep(time.Duration(rand.Intn(1500)+300) * time.Millisecond)
					cancelRide(rideID, token)
				}
			}
		}
	}
}

// driverWorker — обновляет локацию, иногда уходит оффлайн и возвращается
func driverWorker(stop <-chan struct{}, id, token string) {
	locTicker    := time.NewTicker(time.Duration(rand.Intn(2000)+1000) * time.Millisecond)
	toggleTicker := time.NewTicker(time.Duration(rand.Intn(30)+20) * time.Second)
	defer locTicker.Stop()
	defer toggleTicker.Stop()

	online := true
	for {
		select {
		case <-stop:
			if online {
				driverOffline(id, token)
			}
			return
		case <-locTicker.C:
			if online {
				driverLocation(id, token)
			}
		case <-toggleTicker.C:
			if online {
				driverOffline(id, token)
				online = false
				logInfo(fmt.Sprintf("driver %s went OFFLINE", id[:8]))
			} else {
				driverOnline(id, token)
				online = true
				logInfo(fmt.Sprintf("driver %s went ONLINE", id[:8]))
			}
		}
	}
}

// badAuthWorker — постоянно генерирует 4xx запросы к auth сервису
func badAuthWorker(stop <-chan struct{}, validEmails []string) {
	t := time.NewTicker(time.Duration(rand.Intn(1500)+500) * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			switch rand.Intn(3) {
			case 0:
				// несуществующий email
				post("/auth/login", map[string]any{
					"email": randOf(fakeEmails), "password": password,
				}, "")
			case 1:
				// неверный пароль к реальному email
				post("/auth/login", map[string]any{
					"email": randOf(validEmails), "password": randOf(badPasswords),
				}, "")
			case 2:
				// GET /auth/me с невалидным токеном
				get("/auth/me", "invalid.token.here")
			}
		}
	}
}

// authBurstWorker — раз в несколько секунд делает пачку запросов /auth/me
func authBurstWorker(stop <-chan struct{}, tokens []string) {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			count := rand.Intn(8) + 3
			for i := 0; i < count; i++ {
				me(randOf(tokens))
			}
		}
	}
}

// refreshWorker — периодически обновляет токены
func refreshWorker(stop <-chan struct{}, tokens, refreshes map[string]string, mu *sync.Mutex) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			mu.Lock()
			for k, ref := range refreshes {
				if ref == "" {
					continue
				}
				c := refreshCreds(ref)
				if c.access != "" {
					tokens[k] = c.access
					refreshes[k] = c.refresh
				}
			}
			mu.Unlock()
		}
	}
}

// newDriverWorker — каждые N секунд регистрирует нового водителя и включает его онлайн
func newDriverWorker(stop <-chan struct{}) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	n := 0
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			n++
			suffix := time.Now().Unix()
			email := fmt.Sprintf("newdriver%d_%d@demo.com", n, suffix)
			phone := fmt.Sprintf("+7700999%04d", n%10000)
			id := register(fmt.Sprintf("New Driver %d", n), phone, email)
			if id == "" {
				continue
			}
			c := loginAs(email)
			if c.access == "" {
				continue
			}
			registerDriver(id)
			driverOnline(id, c.access)
			logInfo(fmt.Sprintf("new driver #%d registered and online (id=%s)", n, id[:8]))
			// Работает 30-60 секунд, потом уходит оффлайн
			go func(dID, tok string) {
				delay := time.Duration(rand.Intn(30)+30) * time.Second
				time.Sleep(delay)
				driverOffline(dID, tok)
				logInfo(fmt.Sprintf("new driver %s went offline after %.0fs", dID[:8], delay.Seconds()))
			}(id, c.access)
		}
	}
}

// wsWorker — устанавливает WebSocket соединение и держит его живым
// Протокол: подключиться → отправить {"type":"auth","token":"Bearer <jwt>"} → читать события
func wsWorker(stop <-chan struct{}, wsURL, label, token string) {
	for {
		select {
		case <-stop:
			return
		default:
		}

		c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			logWarn(fmt.Sprintf("WS %s dial: %v", label, err))
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		c.SetWriteDeadline(time.Now().Add(4 * time.Second))
		if err := c.WriteJSON(map[string]string{
			"type":  "auth",
			"token": "Bearer " + token,
		}); err != nil {
			c.Close()
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}
		c.SetWriteDeadline(time.Time{})

		logOk(fmt.Sprintf("WS %s connected", label))

		connDone := make(chan struct{})
		go func() {
			defer close(connDone)
			for {
				_, msg, err := c.ReadMessage()
				if err != nil {
					return
				}
				logInfo(fmt.Sprintf("WS %s ← %s", label, string(msg)))
			}
		}()

		select {
		case <-stop:
			_ = c.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
				time.Now().Add(time.Second),
			)
			c.Close()
			return
		case <-connDone:
			c.Close()
			logWarn(fmt.Sprintf("WS %s dropped, reconnecting in 3s", label))
			select {
			case <-stop:
				return
			case <-time.After(3 * time.Second):
			}
		}
	}
}

// statsWorker — печатает сводку каждые 10 секунд
func statsWorker(stop <-chan struct{}, start time.Time) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			elapsed := time.Since(start).Round(time.Second)
			logInfo(fmt.Sprintf(
				"[ STATS ] uptime=%s  requests=%d  rides=%d  errors=%d",
				elapsed, totalReqs.Load(), totalRides.Load(), totalErrors.Load(),
			))
		}
	}
}

// ── Setup ─────────────────────────────────────────────────────────────────────

type user struct {
	key    string
	name   string
	phone  string
	role   string // "passenger" | "driver"
}

var userDefs = []user{
	{"p1", "Alice Bekova", "+77001110001", "passenger"},
	{"p2", "Bob Seitkali", "+77001110002", "passenger"},
	{"p3", "Cara Nurlan",  "+77001110003", "passenger"},
	{"p4", "David Abenov", "+77001110004", "passenger"},
	{"p5", "Eva Jumanova", "+77001110005", "passenger"},
	{"d1", "Carlos Ibr",   "+77009990001", "driver"},
	{"d2", "Diana Mak",    "+77009990002", "driver"},
	{"d3", "Erik Ospanov", "+77009990003", "driver"},
	{"d4", "Fatima Erg",   "+77009990004", "driver"},
}

func setup() (ids, tokens, refreshes map[string]string) {
	suffix := time.Now().Unix()
	ids = make(map[string]string)
	tokens = make(map[string]string)
	refreshes = make(map[string]string)

	emails := make(map[string]string)
	for _, u := range userDefs {
		emails[u.key] = fmt.Sprintf("%s_%d@demo.com", u.key, suffix)
	}

	section("1/3  Registering users")
	for _, u := range userDefs {
		id := register(u.name, u.phone, emails[u.key])
		ids[u.key] = id
		if id != "" {
			logInfo(fmt.Sprintf("%-3s %-20s id=%s", u.key, u.name, id))
		}
		time.Sleep(300 * time.Millisecond)
	}

	section("2/3  Logging in")
	for _, u := range userDefs {
		c := loginAs(emails[u.key])
		tokens[u.key] = c.access
		refreshes[u.key] = c.refresh
		time.Sleep(300 * time.Millisecond)
	}

	if tokens["p1"] == "" {
		fmt.Println("\n\033[31mCould not login. Are services running?\033[0m")
		fmt.Println("  docker-compose up -d\n")
		os.Exit(1)
	}

	section("3/3  Setting up drivers")
	for _, u := range userDefs {
		if u.role != "driver" {
			continue
		}
		if ids[u.key] != "" && tokens[u.key] != "" {
			registerDriver(ids[u.key])
			time.Sleep(300 * time.Millisecond)
			driverOnline(ids[u.key], tokens[u.key])
			time.Sleep(300 * time.Millisecond)
		}
	}

	return ids, tokens, refreshes
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	ids, tokens, refreshes := setup()

	section("LOAD LOOP  —  Ctrl+C to stop")
	fmt.Printf("\033[36m  Grafana → %s  (admin / admin)\033[0m\n\n", grafana)

	stop := make(chan struct{})
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; close(stop) }()

	var tokensMu sync.Mutex
	start := time.Now()

	// Собираем списки токенов для воркеров
	passengerKeys := []string{"p1", "p2", "p3", "p4", "p5"}
	driverKeys    := []string{"d1", "d2", "d3", "d4"}

	allTokens := make([]string, 0, len(passengerKeys))
	for _, k := range passengerKeys {
		if tokens[k] != "" {
			allTokens = append(allTokens, tokens[k])
		}
	}

	validEmails := make([]string, 0)
	for _, u := range userDefs {
		suffix := time.Now().Unix()
		validEmails = append(validEmails, fmt.Sprintf("%s_%d@demo.com", u.key, suffix))
	}

	// ── Запускаем горутины ────────────────────────────────────────────────────

	// Пассажиры с разными интервалами (имитируем разную активность)
	intervals := []time.Duration{
		2 * time.Second,
		3 * time.Second,
		4 * time.Second,
		5 * time.Second,
		7 * time.Second,
	}
	for i, k := range passengerKeys {
		if ids[k] == "" || tokens[k] == "" {
			continue
		}
		go passengerWorker(stop, ids[k], tokens[k], intervals[i%len(intervals)])
	}

	// Водители
	for _, k := range driverKeys {
		if ids[k] == "" || tokens[k] == "" {
			continue
		}
		go driverWorker(stop, ids[k], tokens[k])
	}

	// Генератор плохих запросов (4xx)
	go badAuthWorker(stop, validEmails)
	go badAuthWorker(stop, validEmails) // два воркера для большего объёма

	// Пачки запросов /auth/me
	go authBurstWorker(stop, allTokens)

	// Обновление токенов
	go refreshWorker(stop, tokens, refreshes, &tokensMu)

	// Периодическая регистрация новых водителей
	go newDriverWorker(stop)

	// WebSocket соединения (пассажиры + водители)
	for _, k := range passengerKeys {
		if ids[k] != "" && tokens[k] != "" {
			url := wsBase + "/ws/passengers/" + ids[k]
			go wsWorker(stop, url, "passenger-"+k, tokens[k])
		}
	}
	for _, k := range driverKeys {
		if ids[k] != "" && tokens[k] != "" {
			url := wsBase + "/ws/drivers/" + ids[k]
			go wsWorker(stop, url, "driver-"+k, tokens[k])
		}
	}

	// Статистика
	go statsWorker(stop, start)

	// ── Ждём сигнала остановки ────────────────────────────────────────────────
	<-stop

	section("Shutting down")
	elapsed := time.Since(start).Round(time.Second)
	for _, k := range driverKeys {
		if ids[k] != "" && tokens[k] != "" {
			driverOffline(ids[k], tokens[k])
		}
	}
	fmt.Printf("\n  Uptime      : %s\n", elapsed)
	fmt.Printf("  Requests    : %d\n", totalReqs.Load())
	fmt.Printf("  Rides       : %d\n", totalRides.Load())
	fmt.Printf("  Errors (4xx): %d\n\n", totalErrors.Load())
}
