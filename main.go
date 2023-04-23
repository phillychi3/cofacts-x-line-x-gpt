package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"

	"github.com/line/line-bot-sdk-go/v7/linebot"
	"github.com/machinebox/graphql"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}
	bot, err := linebot.New(
		os.Getenv("CHANNEL_SECRET"),
		os.Getenv("CHANNEL_TOKEN"),
	)
	if err != nil {
		log.Fatal(err)
	}
	client := graphql.NewClient("https://api.cofacts.tw/graphql")

	// Setup HTTP Server for receiving requests from LINE platform
	http.HandleFunc("/callback", func(w http.ResponseWriter, req *http.Request) {
		events, err := bot.ParseRequest(req)
		if err != nil {
			if err == linebot.ErrInvalidSignature {
				w.WriteHeader(400)
			} else {
				w.WriteHeader(500)
			}
			return
		}
		for _, event := range events {
			if event.Type == linebot.EventTypeMessage {
				switch message := event.Message.(type) {
				case *linebot.TextMessage:
					msgtext := message.Text
					//graphql

					request := `
					query {
						ListArticles(filter: {
							moreLikeThis: {
							like: "<replace>"
							}
						}) {
							edges {
							node {
								id
								text
								articleType
								createdAt
								articleReplies{
								reply{
									text
								}
								}
								
							}
							}
							totalCount
						}
					}
				`
					msgtext = strings.Replace(msgtext, "\n", "", -1)
					request = strings.Replace(request, "<replace>", msgtext, 1)

					req := graphql.NewRequest(request)

					ctx := context.Background()

					var respData struct {
						ListArticles struct {
							Edges []struct {
								Node struct {
									ID             string
									Text           string
									ArticleType    string
									CreatedAt      string
									ArticleReplies []struct {
										Reply struct {
											Text string
										}
									}
								}
							}
							TotalCount int
						}
					}
					if err := client.Run(ctx, req, &respData); err != nil {
						log.Fatal(err)
					}
					log.Println(respData)

					if _, err = bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage(message.Text)).Do(); err != nil {
						log.Print(err)
					}
				default:
					log.Printf("not text message: %v", message)
				}

			}
		}
	})
	// This is just sample code.
	// For actual use, you must support HTTPS by using `ListenAndServeTLS`, a reverse proxy or something else.
	if err := http.ListenAndServe(":"+os.Getenv("PORT"), nil); err != nil {
		log.Fatal(err)
	}
}
