package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DelayBetweenPages mirrors the JS script's 18s delay to avoid getting blocked.
const DelayBetweenPages = 18 * time.Second

// RequestTimeout bounds each individual HTTP request.
const RequestTimeout = 30 * time.Second

// apiResponse models the pieces of the Daraz API response we care about.
type apiResponse struct {
	MainInfo struct {
		PageSize     string `json:"pageSize"`
		TotalResults string `json:"totalResults"`
	} `json:"mainInfo"`
	Mods struct {
		ListItems []rawItem `json:"listItems"`
	} `json:"mods"`
}

// rawItem mirrors the fields pulled from each listing item in the JS script.
// Numeric-looking fields are kept as strings since the API is loosely typed
// (the original script ran parseInt/parseFloat on them); we convert when
// writing to CSV.
type rawItem struct {
	Nid             string   `json:"nid"`
	Name            string   `json:"name"`
	ItemId          string   `json:"itemId"`
	PriceShow       string   `json:"priceShow"`
	Discount        string   `json:"discount"`
	RatingScore     string   `json:"ratingScore"`
	Review          string   `json:"review"`
	Location        string   `json:"location"`
	Description     []string `json:"description"`
	SellerName      string   `json:"sellerName"`
	SellerId        string   `json:"sellerId"`
	BrandName       string   `json:"brandName"`
	BrandId         string   `json:"brandId"`
	Price           string   `json:"price"`
	InStock         bool     `json:"inStock"`
	ItemSoldCntShow string   `json:"itemSoldCntShow"`
	OriginalPrice   string   `json:"originalPrice"`
	ItemUrl         string   `json:"itemUrl"`
}

var csvHeader = []string{
	"id", "name", "itemId", "priceShow", "discount", "ratingScore", "review",
	"location", "description", "sellerName", "sellerId", "brandName", "brandId",
	"price", "inStock", "itemSoldCntShow", "originalPrice", "itemUrl", "page_no",
	"scraped_date",
}

var httpClient = &http.Client{Timeout: RequestTimeout}

// errLog is the logger for warnings/errors. It writes to both the monthly
// log file and stderr, so problems are still visible when run interactively.
var errLog *log.Logger

// loadDotEnv reads simple KEY=VALUE pairs from a .env file into the process
// environment. It does not overwrite variables already set in the
// environment, and silently does nothing if the file doesn't exist.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // no .env file present; that's fine, rely on real env vars
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)

		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
}

// getEnv returns the environment variable's value, or def if unset/empty.
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func fetchPage(pageNo int) (*apiResponse, error) {
	url := fmt.Sprintf("https://www.daraz.com.np/computing/?ajax=true&page=%d", pageNo)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// A browser-like UA helps avoid being blocked, same spirit as the delay in the JS version.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; daraz-scraper/1.0)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for page %d", resp.StatusCode, pageNo)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var parsed apiResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse JSON for page %d: %w", pageNo, err)
	}
	return &parsed, nil
}

// toRow converts a rawItem into a CSV row, applying the same int/float
// coercions the original JS script performed.
func toRow(item rawItem, pageNo int, scrapedDate string) []string {
	ratingScore := ""
	if f, err := strconv.ParseFloat(item.RatingScore, 64); err == nil {
		ratingScore = strconv.FormatFloat(f, 'f', 5, 64)
	}

	price := intOrEmpty(item.Price)
	itemSoldCntShow := intOrEmpty(item.ItemSoldCntShow)
	originalPrice := intOrEmpty(item.OriginalPrice)

	return []string{
		fmt.Sprintf("%s-%s", item.Nid, scrapedDate), // id
		item.Name,
		item.ItemId,
		item.PriceShow,
		item.Discount,
		ratingScore,
		item.Review,
		item.Location,
		strings.Join(item.Description, ","),
		item.SellerName,
		item.SellerId,
		item.BrandName,
		item.BrandId,
		price,
		strconv.FormatBool(item.InStock),
		itemSoldCntShow,
		originalPrice,
		item.ItemUrl,
		strconv.Itoa(pageNo),
		scrapedDate,
	}
}

// intOrEmpty parses a string as an integer like JS's parseInt, returning ""
// if it isn't parseable (parseInt would yield NaN).
func intOrEmpty(s string) string {
	// parseInt in JS stops at the first non-digit, e.g. "123abc" -> 123.
	// strconv.Atoi requires the whole string to be numeric, so trim trailing
	// non-digits to mimic that behavior.
	end := 0
	for end < len(s) && (s[end] == '-' || (s[end] >= '0' && s[end] <= '9')) {
		end++
	}
	if end == 0 {
		return ""
	}
	if n, err := strconv.Atoi(s[:end]); err == nil {
		return strconv.Itoa(n)
	}
	return ""
}

func main() {
	loadDotEnv(".env")

	now := time.Now()
	scrapedDate := now.Format("2006-01-02") // daily, en-CA style YYYY-MM-DD
	logMonth := now.Format("2006-01")       // monthly

	csvDir := getEnv("CSV_OUTPUT_DIR", ".")
	logDir := getEnv("ERROR_LOG_DIR", ".")

	if err := os.MkdirAll(csvDir, 0755); err != nil {
		fmt.Println("Failed to create CSV output directory:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Println("Failed to create error log directory:", err)
		os.Exit(1)
	}

	// --- Set up error log (monthly, appended) ---
	logPath := filepath.Join(logDir, fmt.Sprintf("errors_%s.log", logMonth))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Println("Failed to open error log file:", err)
		os.Exit(1)
	}
	defer logFile.Close()

	errLog = log.New(io.MultiWriter(os.Stderr, logFile), "", log.LstdFlags)

	// --- Set up CSV output (daily) ---
	csvPath := filepath.Join(csvDir, fmt.Sprintf("daraz_%s.csv", scrapedDate))
	f, err := os.Create(csvPath)
	if err != nil {
		errLog.Println("Failed to create CSV file:", err)
		os.Exit(1)
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	if err := writer.Write(csvHeader); err != nil {
		errLog.Println("Failed to write CSV header:", err)
		os.Exit(1)
	}
	writer.Flush()

	// Initial request to determine total page count.
	first, err := fetchPage(1)
	if err != nil {
		errLog.Println("Error on initial request:", err)
		os.Exit(1)
	}

	pageSize, err1 := strconv.Atoi(first.MainInfo.PageSize)
	totalResults, err2 := strconv.Atoi(first.MainInfo.TotalResults)
	if err1 != nil || err2 != nil || pageSize == 0 {
		errLog.Println("Could not determine total pages (invalid pageSize/totalResults)")
		os.Exit(1)
	}

	totalPages := totalResults / pageSize
	fmt.Printf("Total Page %d\n", totalPages)

	for page := 1; page < totalPages; page++ {
		resp, err := fetchPage(page)
		if err != nil {
			errLog.Printf("Error fetching page %d: %v\n", page, err)
			time.Sleep(DelayBetweenPages)
			continue
		}

		for _, item := range resp.Mods.ListItems {
			row := toRow(item, page, scrapedDate)
			if err := writer.Write(row); err != nil {
				errLog.Printf("Error writing row on page %d: %v\n", page, err)
			}
		}
		writer.Flush()

		fmt.Printf("Page:%d Done\n", page)
		time.Sleep(DelayBetweenPages)
	}

	fmt.Println("Scraping complete. Output written to", csvPath)
}
