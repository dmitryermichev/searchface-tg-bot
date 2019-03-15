// Телеграм бот для поиска на searchface.ru
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/antonholmquist/jason"

	tb "gopkg.in/tucnak/telebot.v2"
)

// TelegramToken - token for telegram bot
var telegramToken = flag.String("token", "", "telegram token")

type searchFacesImageItem struct {
	score float64
	url   string
}

func main() {
	// Parsing command line arguments
	flag.Parse()
	if *telegramToken == "" {
		log.Fatal("token argument is mandatory")
	}
	b, err := tb.NewBot(tb.Settings{
		Token:  *telegramToken,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})

	if err != nil {
		log.Fatal(err)
		return
	}

	b.Handle(tb.OnText, func(m *tb.Message) {
		b.Send(m.Sender, "Отправьте фотографию для поиска")
	})

	b.Handle(tb.OnPhoto, func(m *tb.Message) {
		photoFilename := os.TempDir() + string(os.PathSeparator) + strconv.FormatInt(time.Now().UnixNano(), 10)
		err := b.Download(&m.Photo.File, photoFilename)
		if err != nil {
			log.Println(err)
		}
		searchResult, err := search(photoFilename)
		if err != nil {
			b.Send(m.Sender, fmt.Sprintf("Ошибка обработки запроса %q", err.Error()))
		}

		album, err := createMessage(searchResult)
		if err != nil {
			_, err := b.Send(m.Sender, err.Error())
			if err != nil {
				log.Println("Error while sending message" + err.Error())
			}
			return
		}

		if _, err := b.SendAlbum(m.Sender, album[:10]); err != nil {
			log.Println("Error while sending album " + err.Error())
		}
	})

	b.Start()
}

func createMessage(searchResult io.ReadCloser) (tb.Album, error) {
	var album tb.Album

	parseResult, err := parse(searchResult)
	if err != nil {
		return nil, err
	}
	for _, item := range parseResult {
		album = append(album, &tb.Photo{
			File:    tb.FromURL(item.url),
			Caption: fmt.Sprintf("Score %f", item.score),
		})

	}

	return album, nil
}

func parse(jsonReader io.ReadCloser) ([]searchFacesImageItem, error) {
	defer jsonReader.Close()

	searchResultBytes, _ := ioutil.ReadAll(jsonReader)
	// Такая обработка ошибок взята из searchface.ru
	if len(searchResultBytes) <= 30 {
		return nil, fmt.Errorf("%s", string(searchResultBytes))
	}
	jsonValue, err := jason.NewValueFromBytes(searchResultBytes)
	if err != nil {
		return nil, err
	}

	jsonArray, err := jsonValue.Array()
	if err != nil {
		return nil, err
	}

	var items []searchFacesImageItem

	for _, item := range jsonArray {
		itemArray, err := item.Array()
		if err != nil {
			return nil, err
		}

		scoreElement := itemArray[0]
		score, _ := scoreElement.Float64()

		imageArray, _ := itemArray[1].Array()
		imageItem, _ := imageArray[0].Array()
		imageURL, _ := imageItem[0].String()

		items = append(items, searchFacesImageItem{
			score: score,
			url:   imageURL,
		})
	}
	return items, nil
}

func search(filename string) (result io.ReadCloser, err error) {

	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("upl", filename)

	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}
	defer writer.Close()

	request, err := http.NewRequest("POST", "http://searchface.ru/request/", body)
	if err != nil {
		return nil, err
	}

	request.Header.Add("Content-Type", writer.FormDataContentType())
	client := &http.Client{}

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	return response.Body, nil
}
