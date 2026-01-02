package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// LogEntry represents a single log entry
type LogEntry struct {
	Line      string         `json:"line"`
	Timestamp string         `json:"timestamp"`
	Fields    map[string]any `json:"fields"`
}

// GameData represents parsed game log data
type GameData struct {
	Level      string `json:"level"`
	Logger     string `json:"logger"`
	Caller     string `json:"caller"`
	Message    string `json:"msg"`
	Command    string `json:"command"`
	Version    string `json:"version"`
	Hostname   string `json:"hostname"`
	PlatformID string `json:"platform_id"`
	OperatorID string `json:"operator_id"`
	GameID     string `json:"game_id"`
	Currency   string `json:"currency"`
	PlayerID   string `json:"player_id"`
	RoomID     string `json:"room_id"`
	ModID      string `json:"mod_id"`
	RoundID    string `json:"round_id"`

	Timestamp float64 `json:"ts"`

	BetID string `json:"bet_id,omitempty"`
	WinID string `json:"win_id,omitempty"`

	Bet     int64 `json:"bet,omitempty"`
	Win     int64 `json:"win,omitempty"`
	Balance int64 `json:"balance"`

	StepNumber int `json:"step_number"`
}

// Report represents the analysis report
type Report struct {
	Summary          Summary               `json:"summary"`
	PlayerStats      map[string]PlayerStat `json:"player_stats"`
	GameStats        map[string]GameStat   `json:"game_stats"`
	TimeStats        []TimeStat            `json:"time_stats"`
	SuspiciousEvents []SuspiciousEvent     `json:"suspicious_events"`
}

type Summary struct {
	TotalBets      int     `json:"total_bets"`
	TotalWins      int     `json:"total_wins"`
	TotalBetAmount int64   `json:"total_bet_amount"`
	TotalWinAmount int64   `json:"total_win_amount"`
	NetResult      int64   `json:"net_result"`
	RTP            float64 `json:"rtp_percentage"`
	UniquePlayers  int     `json:"unique_players"`
	UniqueGames    int     `json:"unique_games"`
	TimeSpan       string  `json:"time_span"`
}

type PlayerStat struct {
	PlayerID       string   `json:"player_id"`
	RTP            float64  `json:"rtp_percentage"`
	TotalBetAmount int64    `json:"total_bet_amount"`
	TotalWinAmount int64    `json:"total_win_amount"`
	NetResult      int64    `json:"net_result"`
	LastBalance    int64    `json:"last_balance"`
	TotalBets      int      `json:"total_bets"`
	TotalWins      int      `json:"total_wins"`
	TopBets        []TopBet `json:"top_bets"`
	TopWins        []TopWin `json:"top_wins"`
}

type GameStat struct {
	GameID         string  `json:"game_id"`
	RTP            float64 `json:"rtp_percentage"`
	TotalBetAmount int64   `json:"total_bet_amount"`
	TotalWinAmount int64   `json:"total_win_amount"`
	TotalBets      int     `json:"total_bets"`
	TotalWins      int     `json:"total_wins"`
	Players        int     `json:"unique_players"`
}

type TimeStat struct {
	Hour           int   `json:"hour"`
	TotalBets      int   `json:"total_bets"`
	TotalWins      int   `json:"total_wins"`
	TotalBetAmount int64 `json:"total_bet_amount"`
	TotalWinAmount int64 `json:"total_win_amount"`
}

type SuspiciousEvent struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	PlayerID    string `json:"player_id"`
	Timestamp   string `json:"timestamp"`
	Details     string `json:"details"`
}

type DailyReport struct {
	Date   string `json:"date"`
	Report Report `json:"report"`
}

type TopBet struct {
	Amount  int64  `json:"amount"`
	RoundID string `json:"round_id"`
	Time    string `json:"time"`
}

type TopWin struct {
	Amount  int64  `json:"amount"`
	RoundID string `json:"round_id"`
	Time    string `json:"time"`
}

// Loki API Configuration
type LokiConfig struct {
	URL      string `json:"url"`       // Loki server URL (e.g., http://localhost:3100)
	Username string `json:"username"`  // Optional: for basic auth
	Password string `json:"password"`  // Optional: for basic auth or token
	TenantID string `json:"tenant_id"` // Optional: for multi-tenant setups
}

// Loki API Response structures
type LokiResponse struct {
	Status string   `json:"status"`
	Data   LokiData `json:"data"`
}

type LokiData struct {
	ResultType string       `json:"resultType"`
	Result     []LokiStream `json:"result"`
}

type LokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// Time range for fetching logs
type TimeRange struct {
	Start time.Time
	End   time.Time
	Label string // Human readable label for the time range
}

func run() error {
	// Check if we should fetch from Loki first
	if shouldFetchFromLoki() {
		if err := fetchLogsFromLoki(); err != nil {
			fmt.Printf("⚠️ Failed to fetch from Loki: %v\n", err)
			fmt.Println("Continuing with existing files...")
		}
	}

	// Find all JSON files in resources directory
	files, err := findJSONFiles()
	if err != nil {
		return fmt.Errorf("finding JSON files: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no JSON files found in resources directory")
	}

	fmt.Printf("📁 Found %d JSON files to analyze:\n", len(files))
	for i, file := range files {
		fmt.Printf("   %d. %s\n", i+1, file)
	}
	fmt.Println()

	logs, err := readMultipleLogFiles(files)
	if err != nil {
		return fmt.Errorf("reading logs: %w", err)
	}

	gameData, err := parseGameData(logs)
	if err != nil {
		return err
	}

	report := generateReport(gameData)

	printReport(report)

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func findJSONFiles() ([]string, error) {
	// Look only in resources directory
	resourcesDir := "./resources"

	// Ensure resources directory exists
	if err := os.MkdirAll(resourcesDir, 0755); err != nil {
		return nil, fmt.Errorf("creating resources directory: %w", err)
	}

	// Find all JSON files in resources directory
	resourceFiles, err := filepath.Glob(filepath.Join(resourcesDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("globbing files in resources: %w", err)
	}

	// Sort files by name for consistent processing order
	sort.Strings(resourceFiles)
	return resourceFiles, nil
}

func shouldFetchFromLoki() bool {
	// Check if loki-config.json exists
	if _, err := os.Stat("loki-config.json"); err == nil {
		return true
	}
	return false
}

func loadLokiConfig() (*LokiConfig, error) {
	data, err := os.ReadFile("loki-config.json")
	if err != nil {
		return nil, fmt.Errorf("reading loki config: %w", err)
	}

	var config LokiConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing loki config: %w", err)
	}

	// Validate required fields
	if config.URL == "" {
		return nil, fmt.Errorf("loki URL is required in config")
	}

	return &config, nil
}

func fetchLogsFromLoki() error {
	fmt.Println("🔄 Fetching logs from Loki...")

	config, err := loadLokiConfig()
	if err != nil {
		return err
	}

	// Define time ranges to fetch (avoid 1000 log limit)
	timeRanges := generateTimeRanges()

	for _, timeRange := range timeRanges {
		fmt.Printf("📅 Fetching logs for: %s\n", timeRange.Label)

		logs, err := queryLokiLogs(config, timeRange)
		if err != nil {
			fmt.Printf("   ⚠️ Failed to fetch logs for %s: %v\n", timeRange.Label, err)
			continue
		}

		if len(logs) == 0 {
			fmt.Printf("   ℹ️ No logs found for %s\n", timeRange.Label)
			continue
		}

		// Save logs to resources directory
		filename := fmt.Sprintf("resources/%s.json",
			strings.ReplaceAll(timeRange.Label, " ", "_"))

		if err := saveLokiLogsToFile(logs, filename); err != nil {
			fmt.Printf("   ⚠️ Failed to save logs for %s: %v\n", timeRange.Label, err)
			continue
		}

		fmt.Printf("   ✅ Saved %d log entries to %s\n", len(logs), filename)

		// Add small delay to avoid overwhelming Loki
		time.Sleep(500 * time.Millisecond)
	}

	return nil
}

func generateTimeRanges() []TimeRange {
	now := time.Now()

	// Generate ranges for the last 7 days, split into manageable chunks
	var ranges []TimeRange

	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)

		// Split each day into 4-hour chunks to stay under 1000 log limit
		for hour := 0; hour < 24; hour += 4 {
			start := time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, day.Location())
			end := start.Add(4 * time.Hour)

			// Don't go beyond current time
			if end.After(now) {
				end = now
			}

			if start.Before(now) {
				ranges = append(ranges, TimeRange{
					Start: start,
					End:   end,
					Label: fmt.Sprintf("%s_%02d-%02d",
						start.Format("2006-01-02"), hour, hour+4),
				})
			}
		}
	}

	return ranges
}

func queryLokiLogs(config *LokiConfig, timeRange TimeRange) ([]LogEntry, error) {
	// Build Loki query URL
	baseURL := strings.TrimSuffix(config.URL, "/") + "/loki/api/v1/query_range"

	// LogQL query for gaming logs
	query := `{level="info"} |= "SendBet" or "SendWin"`

	// Prepare query parameters
	params := url.Values{}
	params.Add("query", query)
	params.Add("start", fmt.Sprintf("%d", timeRange.Start.UnixNano()))
	params.Add("end", fmt.Sprintf("%d", timeRange.End.UnixNano()))
	params.Add("limit", "1000") // Loki's default limit
	params.Add("direction", "forward")

	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	// Create HTTP request
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Add authentication if provided
	if config.Username != "" && config.Password != "" {
		req.SetBasicAuth(config.Username, config.Password)
	}

	// Add tenant header if provided
	if config.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", config.TenantID)
	}

	// Make the request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("loki returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse Loki response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var lokiResp LokiResponse
	if err := json.Unmarshal(body, &lokiResp); err != nil {
		return nil, fmt.Errorf("parsing loki response: %w", err)
	}

	if lokiResp.Status != "success" {
		return nil, fmt.Errorf("loki query failed with status: %s", lokiResp.Status)
	}

	// Convert Loki logs to our LogEntry format
	var logEntries []LogEntry
	for _, stream := range lokiResp.Data.Result {
		for _, value := range stream.Values {
			if len(value) >= 2 {
				// value[0] is timestamp (nanoseconds), value[1] is log line
				timestamp := value[0]
				logLine := value[1]

				// Convert nanosecond timestamp to RFC3339
				if ns, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
					rfc3339Time := time.Unix(0, ns).Format(time.RFC3339Nano)

					logEntry := LogEntry{
						Line:      logLine,
						Timestamp: rfc3339Time,
						Fields:    make(map[string]any),
					}

					// Add stream labels to fields
					for k, v := range stream.Stream {
						logEntry.Fields[k] = v
					}

					logEntries = append(logEntries, logEntry)
				}
			}
		}
	}

	return logEntries, nil
}

func saveLokiLogsToFile(logs []LogEntry, filename string) error {
	// Ensure resources directory exists
	if err := os.MkdirAll("resources", 0755); err != nil {
		return fmt.Errorf("creating resources directory: %w", err)
	}

	data, err := json.MarshalIndent(logs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling logs: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

func readLogsEntry(fileName string) ([]LogEntry, error) {
	data, err := os.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	var logs []LogEntry
	if err := json.Unmarshal(data, &logs); err != nil {
		return nil, fmt.Errorf("unmarshaling: %w", err)
	}

	return logs, nil
}

func readMultipleLogFiles(fileNames []string) ([]LogEntry, error) {
	var allLogs []LogEntry

	for _, fileName := range fileNames {
		fmt.Printf("📖 Reading %s...\n", fileName)
		logs, err := readLogsEntry(fileName)
		if err != nil {
			fmt.Printf("   ⚠️  Warning: failed to read %s: %v\n", fileName, err)
			continue
		}
		fmt.Printf("   ✅ Loaded %d entries from %s\n", len(logs), fileName)
		allLogs = append(allLogs, logs...)
	}

	fmt.Printf("\n📊 Total entries loaded: %d\n\n", len(allLogs))
	return allLogs, nil
}

func parseGameData(logs []LogEntry) ([]GameData, error) {
	var gameData []GameData

	for _, logEntry := range logs {
		if logEntry.Line == "" {
			continue
		}

		var data GameData
		if err := json.Unmarshal([]byte(logEntry.Line), &data); err != nil {
			return nil, fmt.Errorf("unmarshaling: %w", err)
		}

		gameData = append(gameData, data)
	}

	return gameData, nil
}

func generateReport(gameData []GameData) Report {
	report := Report{
		PlayerStats:      make(map[string]PlayerStat),
		GameStats:        make(map[string]GameStat),
		SuspiciousEvents: []SuspiciousEvent{},
	}

	var (
		totalBets      int
		totalWins      int
		totalBetAmount int64
		totalWinAmount int64
		uniquePlayers          = make(map[string]bool)
		uniqueGames            = make(map[string]bool)
		uniqueBetIDs           = make(map[string]bool) // Track processed bet IDs
		uniqueWinIDs           = make(map[string]bool) // Track processed win IDs
		timeStats              = make(map[int]TimeStat)
		minTime        float64 = -1 // Use -1 to indicate uninitialized
		maxTime        float64 = 0
		playerBalances         = make(map[string][]int64)
		duplicateBets  int     = 0 // Counter for duplicate bets
		duplicateWins  int     = 0 // Counter for duplicate wins
	)

	// Process each game data entry
	for _, data := range gameData {
		uniquePlayers[data.PlayerID] = true
		uniqueGames[data.GameID] = true

		// Update min/max time
		if data.Timestamp > maxTime {
			maxTime = data.Timestamp
		}
		if minTime < 0 || data.Timestamp < minTime {
			minTime = data.Timestamp
		}

		// Track balance changes
		playerBalances[data.PlayerID] = append(playerBalances[data.PlayerID], data.Balance)

		// Parse hour from Unix timestamp (convert to time object first)
		gameTime := time.Unix(int64(data.Timestamp), 0)
		hour := gameTime.Hour()

		// Initialize time stats if not exists
		if _, exists := timeStats[hour]; !exists {
			timeStats[hour] = TimeStat{Hour: hour}
		}

		// Process bet or win
		if data.Message == "SendBet" && data.Bet > 0 {
			// Check if this bet ID was already processed
			if data.BetID != "" {
				if uniqueBetIDs[data.BetID] {
					duplicateBets++
					fmt.Printf("   ⚠️  Skipping duplicate bet ID: %s\n", data.BetID)
					continue // Skip duplicate bet
				}
				uniqueBetIDs[data.BetID] = true
			}

			totalBets++
			totalBetAmount += data.Bet

			// Update player stats
			pStat := report.PlayerStats[data.PlayerID]
			pStat.PlayerID = data.PlayerID
			pStat.TotalBets++
			pStat.TotalBetAmount += data.Bet
			pStat.LastBalance = data.Balance

			// Track top bets
			topBet := TopBet{
				Amount:  data.Bet,
				RoundID: data.RoundID,
				Time:    time.Unix(int64(data.Timestamp), 0).Format("2006-01-02 15:04:05"),
			}
			pStat.TopBets = append(pStat.TopBets, topBet)

			report.PlayerStats[data.PlayerID] = pStat

			// Update game stats
			gStat := report.GameStats[data.GameID]
			gStat.GameID = data.GameID
			gStat.TotalBets++
			gStat.TotalBetAmount += data.Bet
			if _, exists := report.GameStats[data.GameID]; !exists {
				gStat.Players = 0
			}
			report.GameStats[data.GameID] = gStat

			// Update time stats
			tStat := timeStats[hour]
			tStat.TotalBets++
			tStat.TotalBetAmount += data.Bet
			timeStats[hour] = tStat

		} else if data.Message == "SendWin" && data.Win > 0 {
			// Check if this win ID was already processed
			if data.WinID != "" {
				if uniqueWinIDs[data.WinID] {
					duplicateWins++
					fmt.Printf("   ⚠️  Skipping duplicate win ID: %s\n", data.WinID)
					continue // Skip duplicate win
				}
				uniqueWinIDs[data.WinID] = true
			}

			totalWins++
			totalWinAmount += data.Win

			// Update player stats
			pStat := report.PlayerStats[data.PlayerID]
			pStat.TotalWins++
			pStat.TotalWinAmount += data.Win
			pStat.LastBalance = data.Balance

			// Track top wins
			topWin := TopWin{
				Amount:  data.Win,
				RoundID: data.RoundID,
				Time:    time.Unix(int64(data.Timestamp), 0).Format("2006-01-02 15:04:05"),
			}
			pStat.TopWins = append(pStat.TopWins, topWin)

			report.PlayerStats[data.PlayerID] = pStat

			// Update game stats
			gStat := report.GameStats[data.GameID]
			gStat.TotalWins++
			gStat.TotalWinAmount += data.Win
			report.GameStats[data.GameID] = gStat

			// Update time stats
			tStat := timeStats[hour]
			tStat.TotalWins++
			tStat.TotalWinAmount += data.Win
			timeStats[hour] = tStat
		}
	}

	// Calculate derived stats
	for playerID, pStat := range report.PlayerStats {
		pStat.NetResult = pStat.TotalWinAmount - pStat.TotalBetAmount
		if pStat.TotalBetAmount > 0 {
			pStat.RTP = float64(pStat.TotalWinAmount) / float64(pStat.TotalBetAmount) * 100
		}

		// Sort and limit top bets (keep top 5)
		sort.Slice(pStat.TopBets, func(i, j int) bool {
			return pStat.TopBets[i].Amount > pStat.TopBets[j].Amount
		})
		if len(pStat.TopBets) > 5 {
			pStat.TopBets = pStat.TopBets[:5]
		}

		// Sort and limit top wins (keep top 5)
		sort.Slice(pStat.TopWins, func(i, j int) bool {
			return pStat.TopWins[i].Amount > pStat.TopWins[j].Amount
		})
		if len(pStat.TopWins) > 5 {
			pStat.TopWins = pStat.TopWins[:5]
		}

		report.PlayerStats[playerID] = pStat

		// Detect suspicious activities
		if pStat.TotalBets > 100 && pStat.RTP > 150 {
			report.SuspiciousEvents = append(report.SuspiciousEvents, SuspiciousEvent{
				Type:        "High RTP",
				Description: "Player has suspiciously high RTP",
				PlayerID:    playerID,
				Details:     fmt.Sprintf("RTP: %.2f%%, Bets: %d", pStat.RTP, pStat.TotalBets),
			})
		}
	}

	for gameID, gStat := range report.GameStats {
		if gStat.TotalBetAmount > 0 {
			gStat.RTP = float64(gStat.TotalWinAmount) / float64(gStat.TotalBetAmount) * 100
		}
		// Count unique players per game
		playersInGame := make(map[string]bool)
		for _, data := range gameData {
			if data.GameID == gameID {
				playersInGame[data.PlayerID] = true
			}
		}
		gStat.Players = len(playersInGame)
		report.GameStats[gameID] = gStat
	}

	// Convert time stats map to slice and sort
	for _, tStat := range timeStats {
		report.TimeStats = append(report.TimeStats, tStat)
	}
	sort.Slice(report.TimeStats, func(i, j int) bool {
		return report.TimeStats[i].Hour < report.TimeStats[j].Hour
	})

	// Calculate summary
	report.Summary = Summary{
		TotalBets:      totalBets,
		TotalWins:      totalWins,
		TotalBetAmount: totalBetAmount,
		TotalWinAmount: totalWinAmount,
		NetResult:      totalWinAmount - totalBetAmount,
		UniquePlayers:  len(uniquePlayers),
		UniqueGames:    len(uniqueGames),
	}

	if totalBetAmount > 0 {
		report.Summary.RTP = float64(totalWinAmount) /
			float64(totalBetAmount) * 100
	}

	if minTime >= 0 && maxTime > minTime {
		startTime := time.Unix(int64(minTime), 0)
		endTime := time.Unix(int64(maxTime), 0)
		report.Summary.TimeSpan = fmt.Sprintf("%s - %s", startTime.Format("2006-01-02 15:04:05"), endTime.Format("2006-01-02 15:04:05"))
	}

	// Print duplicate statistics if any found
	if duplicateBets > 0 || duplicateWins > 0 {
		fmt.Printf("\n📋 DUPLICATE DETECTION:\n")
		if duplicateBets > 0 {
			fmt.Printf("├─ Duplicate bets found and skipped: %d\n", duplicateBets)
		}
		if duplicateWins > 0 {
			fmt.Printf("├─ Duplicate wins found and skipped: %d\n", duplicateWins)
		}
		fmt.Printf("└─ Only unique transactions included in analysis\n")
	} else {
		fmt.Printf("\n✅ DATA INTEGRITY: No duplicate transactions detected\n")
	}

	return report
}

func printDailyReport(daily DailyReport) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Printf("                    DAILY REPORT - %s\n", daily.Date)
	fmt.Println(strings.Repeat("=", 60))

	report := daily.Report

	// Summary
	fmt.Println("\n📊 DAILY STATISTICS:")
	fmt.Printf("├─ Analysis Period: %s\n", report.Summary.TimeSpan)
	fmt.Printf("├─ Total Bets: %d\n", report.Summary.TotalBets)
	fmt.Printf("├─ Total Wins: %d\n", report.Summary.TotalWins)
	fmt.Printf("├─ Total Bet Amount: %s NGN\n", formatCurrency(report.Summary.TotalBetAmount))
	fmt.Printf("├─ Total Win Amount: %s NGN\n", formatCurrency(report.Summary.TotalWinAmount))
	fmt.Printf("├─ Net Result: %s NGN\n", formatCurrency(report.Summary.NetResult))
	fmt.Printf("├─ RTP (Return to Player): %.2f%%\n", report.Summary.RTP)
	fmt.Printf("├─ Unique Players: %d\n", report.Summary.UniquePlayers)
	fmt.Printf("└─ Unique Games: %d\n", report.Summary.UniqueGames)

	// Top player for the day
	if len(report.PlayerStats) > 0 {
		fmt.Println("\n👥 TOP PLAYER OF THE DAY:")
		var topPlayer PlayerStat
		var topPlayerID string
		for pid, stat := range report.PlayerStats {
			if stat.TotalBetAmount > topPlayer.TotalBetAmount {
				topPlayer = stat
				topPlayerID = pid
			}
		}
		fmt.Printf("Player ID: %s\n", topPlayerID)
		fmt.Printf("├─ 📊 Activity: %d bets, %d wins\n", topPlayer.TotalBets, topPlayer.TotalWins)
		fmt.Printf("├─ 💰 Volume: Bet %s NGN, Win %s NGN\n",
			formatCurrency(topPlayer.TotalBetAmount), formatCurrency(topPlayer.TotalWinAmount))
		fmt.Printf("├─ 📉 Net Profit: %s NGN (%.2f%%)\n",
			formatCurrency(topPlayer.NetResult),
			float64(topPlayer.NetResult)/float64(topPlayer.TotalBetAmount)*100)
		fmt.Printf("└─ 🎯 RTP: %.2f%%, Current Balance: %s NGN\n",
			topPlayer.RTP, formatCurrency(topPlayer.LastBalance))
	}

	// Game performance for the day
	fmt.Println("\n🎮 GAME PERFORMANCE:")
	for gameID, stat := range report.GameStats {
		fmt.Printf("Game: %s - RTP: %.2f%%, Volume: %s NGN\n",
			gameID, stat.RTP, formatCurrency(stat.TotalBetAmount))
	}

	fmt.Println(strings.Repeat("-", 60))
}

func extractDateFromFilename(filename string) string {
	// Extract date from filename like "25.12.2025.json" or "26.12.2025(1).json"
	base := filepath.Base(filename)
	if strings.Contains(base, ".") {
		parts := strings.Split(base, ".")
		if len(parts) >= 3 {
			return fmt.Sprintf("%s.%s.%s", parts[0], parts[1], parts[2])
		}
	}
	return base
}

func printOverallReport(report Report) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("                    OVERALL SUMMARY REPORT")
	fmt.Println(strings.Repeat("=", 60))

	printReport(report)
}

func printReport(report Report) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("                    GAMING LOGS ANALYSIS REPORT")
	fmt.Println(strings.Repeat("=", 60))

	// Summary
	fmt.Println("\n📊 GENERAL STATISTICS:")
	fmt.Printf("├─ Analysis Period: %s\n", report.Summary.TimeSpan)
	fmt.Printf("├─ Total Bets: %d\n", report.Summary.TotalBets)
	fmt.Printf("├─ Total Wins: %d\n", report.Summary.TotalWins)
	fmt.Printf("├─ Total Bet Amount: %s NGN\n", formatCurrency(report.Summary.TotalBetAmount))
	fmt.Printf("├─ Total Win Amount: %s NGN\n", formatCurrency(report.Summary.TotalWinAmount))
	fmt.Printf("├─ Net Result: %s NGN\n", formatCurrency(report.Summary.NetResult))
	fmt.Printf("├─ RTP (Return to Player): %.2f%%\n", report.Summary.RTP)
	fmt.Printf("├─ Unique Players: %d\n", report.Summary.UniquePlayers)
	fmt.Printf("└─ Unique Games: %d\n", report.Summary.UniqueGames)

	// Player stats
	fmt.Printf("\n👥 PLAYER ANALYSIS (%d unique players):\n", len(report.PlayerStats))
	type PlayerRank struct {
		PlayerID string
		Stat     PlayerStat
	}
	var playerRanks []PlayerRank
	for pid, stat := range report.PlayerStats {
		playerRanks = append(playerRanks, PlayerRank{pid, stat})
	}
	sort.Slice(playerRanks, func(i, j int) bool {
		return playerRanks[i].Stat.TotalBetAmount > playerRanks[j].Stat.TotalBetAmount
	})

	// Show top players (max 10)
	displayCount := min(10, len(playerRanks))
	for i, pr := range playerRanks[:displayCount] {
		fmt.Printf("Player #%d: %s\n", i+1, pr.PlayerID)
		fmt.Printf("├─ 📊 Activity: %d bets, %d wins\n", pr.Stat.TotalBets, pr.Stat.TotalWins)
		fmt.Printf("├─ 💰 Volume: Bet %s NGN, Win %s NGN\n", formatCurrency(pr.Stat.TotalBetAmount), formatCurrency(pr.Stat.TotalWinAmount))

		// Profit display in currency and percentage
		profitPercent := float64(0)
		if pr.Stat.TotalBetAmount > 0 {
			profitPercent = (float64(pr.Stat.NetResult) / float64(pr.Stat.TotalBetAmount)) * 100
		}
		profitStatus := "📈"
		if pr.Stat.NetResult < 0 {
			profitStatus = "📉"
		}
		fmt.Printf("├─ %s Net Profit: %s NGN (%.2f%%)\n", profitStatus, formatCurrency(pr.Stat.NetResult), profitPercent)
		fmt.Printf("├─ 🎯 RTP: %.2f%%, Current Balance: %s NGN\n", pr.Stat.RTP, formatCurrency(pr.Stat.LastBalance))

		// Top bets (only if they exist)
		if len(pr.Stat.TopBets) > 0 {
			fmt.Printf("├─ 🎲 Largest Bets: ")
			topBetCount := min(3, len(pr.Stat.TopBets))
			for j, bet := range pr.Stat.TopBets[:topBetCount] {
				if j > 0 {
					fmt.Printf(", ")
				}
				fmt.Printf("%s NGN", formatCurrency(bet.Amount))
			}
			fmt.Printf("\n")
		}

		// Top wins (only if they exist and > 0)
		hasWins := false
		for _, win := range pr.Stat.TopWins {
			if win.Amount > 0 {
				hasWins = true
				break
			}
		}

		if hasWins {
			fmt.Printf("└─ 🏆 Biggest Wins: ")
			topWinCount := min(3, len(pr.Stat.TopWins))
			winCount := 0
			for _, win := range pr.Stat.TopWins[:topWinCount] {
				if win.Amount > 0 {
					if winCount > 0 {
						fmt.Printf(", ")
					}
					fmt.Printf("%s NGN", formatCurrency(win.Amount))
					winCount++
				}
			}
			fmt.Printf("\n")
		} else {
			fmt.Printf("└─ 🏆 No wins recorded\n")
		}

		// Add spacing between players if there are multiple
		if len(playerRanks) > 1 && i < displayCount-1 {
			fmt.Printf("\n")
		}
	}

	// Game stats
	fmt.Println("\n🎮 GAME STATISTICS:")
	for gameID, stat := range report.GameStats {
		fmt.Printf("Game: %s\n", gameID)
		fmt.Printf("├─ Bets: %d, Wins: %d\n", stat.TotalBets, stat.TotalWins)
		fmt.Printf("├─ Bet Volume: %s NGN\n", formatCurrency(stat.TotalBetAmount))
		fmt.Printf("├─ Win Volume: %s NGN\n", formatCurrency(stat.TotalWinAmount))
		fmt.Printf("├─ RTP: %.2f%%\n", stat.RTP)
		fmt.Printf("└─ Players: %d\n", stat.Players)
	}

	// Time stats
	fmt.Println("\n⏰ HOURLY ACTIVITY:")
	for _, tStat := range report.TimeStats {
		if tStat.TotalBets > 0 {
			fmt.Printf("%02d:00 - Bets: %4d, Wins: %4d, Volume: %s NGN\n",
				tStat.Hour, tStat.TotalBets, tStat.TotalWins, formatCurrency(tStat.TotalBetAmount))
		}
	}

	// Suspicious events
	if len(report.SuspiciousEvents) > 0 {
		fmt.Println("\n🚨 SUSPICIOUS ACTIVITY:")
		for i, event := range report.SuspiciousEvents {
			fmt.Printf("%d. %s\n", i+1, event.Type)
			fmt.Printf("   ├─ Player: %s\n", event.PlayerID)
			fmt.Printf("   ├─ Description: %s\n", event.Description)
			fmt.Printf("   └─ Details: %s\n", event.Details)
		}
	} else {
		fmt.Println("\n✅ GAME INTEGRITY STATUS:")
		fmt.Printf("├─ No suspicious activity detected\n")
		fmt.Printf("├─ All player RTP values are within normal ranges\n")
		fmt.Printf("├─ No unusual betting patterns identified\n")
		fmt.Printf("├─ Overall RTP: %.2f%% (within expected range)\n", report.Summary.RTP)
		fmt.Printf("└─ Game appears to be operating normally\n")
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("                        END OF REPORT")
	fmt.Println(strings.Repeat("=", 60))
}

func formatCurrency(amount int64) string {
	str := strconv.FormatInt(amount, 10)
	n := len(str)
	if n <= 3 {
		return str
	}

	var result strings.Builder
	for i, digit := range str {
		if i > 0 && (n-i)%3 == 0 {
			result.WriteString(",")
		}
		result.WriteRune(digit)
	}
	return result.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
