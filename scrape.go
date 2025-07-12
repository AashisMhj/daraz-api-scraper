package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

var currentDate = time.Now().Format("2006-01-02")

type ItemType struct {
	Nid           string   `json:"nid"`
	Name          string   `json:"name"`
	ItemId        string   `json:"itemId"`
	PriceShow     string   `json:"priceShow"`
	Discount      string   `json:"discount"`
	RatingScore   string   `json:"ratingScore"`
	Review        string   `json:"review"`
	Location      string   `json:"location"`
	Description   []string `json:"description"`
	SellerName    string   `json:"sellerName"`
	SellerId      string   `json:"sellerId"`
	BrandName     string   `json:"brandName"`
	BrandId       string   `json:"brandId"`
	Price         string   `json:"price"`
	InStock       bool     `json:"inStock"`
	ItemSold      string   `json:"itemSoldCntShow"`
	OriginalPrice string   `json:"originalPrice"`
	ItemUrl       string   `json:"itemUrl"`
}

type ModType struct {
	ListItems []ItemType `json:"listItems"`
}

type MainInfoType struct {
	PageTitle    string `json:"pageTitle"`
	Page         string `json:"page"`
	PageSize     string `json:"pageSize"`
	TotalResults string `json:"totalResults"`
}

type ResType struct {
	Mods     ModType      `json:"mods"`
	MainInfo MainInfoType `json:"mainInfo"`
}

func fetchData(page_no int) (ResType, error) {
	url := fmt.Sprintf("https://www.daraz.com.np/computing/?ajax=true&page=%d", page_no)
	resp, err := http.Get(url)
	if err != nil {
		return ResType{}, fmt.Errorf("error making GET request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ResType{}, fmt.Errorf("bad status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ResType{}, fmt.Errorf("error reading response body: %v", err)
	}

	var posts ResType
	err = json.Unmarshal(body, &posts)
	if err != nil {
		return ResType{}, fmt.Errorf("error unmarshaling JSON: %v", err)
	}

	return posts, nil
}

func parseIntoOrZero(value string) int64 {
	int_value, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return int_value
}

func parseFloatOrZero(value string) float64 {
	float_value, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return float_value
}

func extractSoldNumber(input string) string {
	re := regexp.MustCompile(`\d+`)
	match := re.FindString(input)
	return match
}

func insertData(datas []ItemType, page_no int, conn clickhouse.Conn) error {
	ctx := clickhouse.Context(context.Background(), clickhouse.WithSettings(clickhouse.Settings{
		"allow_experimental_object_type": "1",
	}))
	batch, err := conn.PrepareBatch(ctx, "INSERT INTO daraz")
	if err != nil {
		return err
	}
	for i := 0; i < len(datas); i++ {
		data := datas[i]
		// fmt.Printf("%f %s \n", parseFloatOrZero(data.RatingScore), data.RatingScore)
		if err := batch.Append(
			data.Nid,
			data.Name,
			data.ItemId,
			data.PriceShow,
			data.Discount,
			parseFloatOrZero(data.RatingScore),
			data.Review,
			data.Location,
			strings.Join(data.Description, " "),
			data.SellerName,
			data.SellerId,
			data.BrandName,
			data.BrandId,
			parseIntoOrZero(data.Price),
			page_no,
			currentDate,
			data.ItemUrl,
			data.InStock,
			parseIntoOrZero(extractSoldNumber(data.ItemSold)),
			parseIntoOrZero(data.OriginalPrice),
		); err != nil {
			return err
		}
	}

	return batch.Send()

}

func main() {
	url := "localhost:9000"
	username := "default"
	password := ""
	db_name := "default"
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{url},
		Auth: clickhouse.Auth{
			Database: db_name,
			Username: username,
			Password: password,
		},
		// Debug:       true,
		DialTimeout: 5 * time.Second,
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
	})
	if err != nil {
		log.Fatalf("failed to connect to database error: %v", err)
	}
	defer conn.Close()
	res, err := fetchData(1)
	if err != nil {
		log.Fatalf("failed to fetch from api: %v", err)
	}
	totalResults, err1 := strconv.ParseInt(res.MainInfo.TotalResults, 10, 64)
	pageSize, err2 := strconv.ParseInt(res.MainInfo.PageSize, 10, 64)
	if err1 != nil || err2 != nil {
		log.Fatalf("failed to parse total results: %s, page size %s", res.MainInfo.TotalResults, res.MainInfo.PageSize)
	}
	totalPages := totalResults / pageSize
	fmt.Printf("Total Pages: %d, result: %d, page size: %d  \n", totalPages, totalResults, pageSize)
	err = insertData(res.Mods.ListItems, 1, conn)
	if err != nil {
		log.Fatalf("failed to insert to db %v", err)
	}
	fmt.Printf("Inserted page 1 \n")

	for i := 2; i <= int(totalPages); i++ {
		res, err := fetchData(i)
		if err != nil {
			fmt.Printf("failed to fetch from api page %d error: %v \n", i, err)
			continue
		}
		err = insertData(res.Mods.ListItems, i, conn)
		if err != nil {
			fmt.Printf("failed to insert to db page %d error: %v \n", i, err)
			continue
		}
		fmt.Printf("Inserted page: %d \n", i)
		time.Sleep(25 * time.Second)
	}

}
