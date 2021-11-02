package main

import (
	// "encoding/json"
	// "flag"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/cavaliercoder/grab"
)

var (
	pathToDownload string = "./imgs"
	pagesQuontity  int    = 5 // Количество поисковых страниц
	// URL адрес
	URL     string = "https://yandex.ru"
	urlPath string = "/images/touch/search"
	sQuery  string = "машинки"
)

type htmlData struct {
	SerpItem struct {
		ImgHref string `json:"img_href"`
	} `json:"serp-item"`
}

func checkErrFatal(err error) {
	if err != nil {
		log.Fatal(err)
	}

}
func getURL(urI, urlPath, sQuery string, page int) string {
	baseURL, err := url.Parse(urI) // получаем и проверяем адрес
	checkErrFatal(err)
	baseURL.Path += urlPath
	v := url.Values{}
	v.Add("text", sQuery)
	p := fmt.Sprintf("%d", page) // Конвертируем Int в String
	v.Add("p", p)
	baseURL.RawQuery = v.Encode() // Добавляем параметры к адресу
	return baseURL.String()
}
func filesDownloader(urls chan string, wg *sync.WaitGroup) {
	for url := range urls {
		resp, err := grab.Get(pathToDownload, url)
		if err != nil {
			fmt.Println("Can't download ", resp.Filename)
		}
		//wg.Done()

	}
}

func findImgURL(url string, wg *sync.WaitGroup, urlsChan chan string) {
	// Request the HTML page.
	res, err := http.Get(url)
	checkErrFatal(err)
	defer func() {
		res.Body.Close()
		wg.Done()
	}()
	if res.StatusCode != 200 {
		log.Fatalf("Wrong status code: %d %s", res.StatusCode, res.Status)
	}
	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatal(err)
	}
	// Find the review items
	sel := doc.Find(".serp-item ")
	//fmt.Printf("type: %T, value: %v\n", sel, sel.Nodes)
	sel.Each(func(i int, s *goquery.Selection) {
		dataBem, ok := s.Attr("data-bem")
		if ok {
			hd := &htmlData{}
			err := json.Unmarshal([]byte(dataBem), hd)
			if err != nil {
				log.Fatal("Cant unmarshal data-bem ", err)
			}
			//wg.Add(1)
			urlsChan <- hd.SerpItem.ImgHref
		}
	})
}
func main() {
	var wg sync.WaitGroup
	urls := make(chan string, 100) // Канал с буфером в 100 строк
	for p := 1; p <= pagesQuontity; p++ {
		url := getURL(URL, urlPath, sQuery, p)
		wg.Add(1)
		go findImgURL(url, &wg, urls)
		go filesDownloader(urls, &wg)
	}
	wg.Wait()
	close(urls)
}
