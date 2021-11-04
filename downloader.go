package main

import (
	// "encoding/json"
	// "flag"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	u "net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/PuerkitoBio/goquery"
	"github.com/lodmev/go/log"
)

func init() {
	flag.StringVar(&sQuery, "q", "", "String with search request")
	flag.StringVar(&baseDir, "p", "./imgs", "Path to downdload imgs")
	flag.BoolVar(&showTrace, "show", false, "Show messages about processing")
}

var (
	pathToDownload, baseDir string
	pagesQuontity           int   = 5 // Количество поисковых страниц
	counter                 int32 = 0
	// URL адрес
	URL       string = "https://yandex.ru"
	urlPath   string = "/images/touch/search"
	sQuery    string
	showTrace bool
)

func resolveTilda(path *string) error {
	p := *path
	hasTilda := false
	u, err := user.Current()
	hdir := u.HomeDir
	if p == "~" {
		hasTilda = true
		p = hdir
	} else if strings.HasPrefix(p, "~/") {
		hasTilda = true
		p = filepath.Join(hdir, p[2:])
	}
	if hasTilda {
		if err == nil {
			*path = p
			return nil
		} else {
			return err
		}
	}
	return nil
}

func createDir(path string, dirName string) (string, error) {
	var err error
	if err = resolveTilda(&path); err != nil {
		return "", err
	}
	p := path
	p += "/"
	p = filepath.Dir(p)
	p, err = filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("can't get absolute path: %s", err)
	}

	p = filepath.Join(p, dirName)
	err = os.MkdirAll(p, 0700)
	if err != nil {
		return "", err

	}
	// If impossible evaluate symlink, just skipping
	t, err := filepath.EvalSymlinks(p)
	if err == nil {
		p = t
	}
	return p, nil

}

type htmlData struct {
	SerpItem struct {
		ImgHref string `json:"img_href"`
	} `json:"serp-item"`
}

func checkErrFatal(err error) {
	if err != nil {
		log.Fatal().Msgf("%s", err)
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
func downloaders(urls chan string, wg *sync.WaitGroup) {
	for url := range urls {
		err := downloadFile(url)
		if err != nil {
			log.Error().Msgf("can't download from url: %s : %s\n", url, err)
		}
		wg.Done()

	}
}
func getExt(mimeType string) (string, error) {
	switch mimeType {
	case "image/jpeg", "image/jpg", "application/octet-stream", "":
		return "jpg", nil
	case "image/png":
		return "png", nil
	case "image/gif":
		return "gif", nil
	default:
		return "", fmt.Errorf("unsupport mime type: %s", mimeType)
	}
}

func downloadFile(url string) error {
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}
	mime := resp.Header.Get("content-type")
	ext, err := getExt(mime)
	if err != nil {
		return err
	}
	fp := filepath.Join(pathToDownload, getNextName(ext))

	// Create the file
	out, err := os.Create(fp)
	if err != nil {
		out.Close()
		return err
	}
	// Writer the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}
	out.Close()
	log.Trace().Msgf("File %s was saved", fp)
	return nil
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
		log.Fatal().Msgf("Wrong status code: %d %s", res.StatusCode, res.Status)
	}
	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		log.Fatal().Msgf("%s", err)
	}
	// Find the review items
	sel := doc.Find(".serp-item ")
	sel.Each(func(i int, s *goquery.Selection) {
		dataBem, ok := s.Attr("data-bem")
		if ok {
			hd := &htmlData{}
			err := json.Unmarshal([]byte(dataBem), hd)
			if err != nil {
				log.Error().Msgf("Cant unmarshal data-bem %s", err)
				return
			}
			urlUnescaped, err := u.QueryUnescape(hd.SerpItem.ImgHref)
			if err != nil {
				return
			}
			wg.Add(1)
			urlsChan <- urlUnescaped
		}
	})
}

func isFlagSet(name string) (isSet bool) {
	isSet = false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			isSet = true
		}
	})
	return
}

func getNextName(ext string) string {
	n := atomic.AddInt32(&counter, 1)
	return fmt.Sprintf("%d.%s", n, ext)

}

func main() {
	flag.Parse()
	if !isFlagSet("q") {
		log.Error().Msg("Need explicitly set flag -q")
		flag.PrintDefaults()
		os.Exit(1)
	}
	logL := "error"
	if showTrace {
		logL = "trace"
	}
	log.SetupLogger(log.LoggerConfig{Level: logL, Human: true})
	var err error
	pathToDownload, err = createDir(baseDir, sQuery)
	if err != nil {
		log.Fatal().Msgf("can't create folder for download: %s", err)
	}
	var wg sync.WaitGroup
	urls := make(chan string, 100) // Канал с буфером в 100 строк
	for p := 1; p <= pagesQuontity; p++ {
		url := getURL(URL, urlPath, sQuery, p)
		wg.Add(1)
		go findImgURL(url, &wg, urls)
		go downloaders(urls, &wg)
	}
	fmt.Println("Starting downloading file to ", pathToDownload)
	wg.Wait()
	close(urls)
	fmt.Printf("Done. Downloaded %d files\n", counter)
}