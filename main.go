package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"

	"github.com/line/line-bot-sdk-go/v7/linebot"
	"github.com/machinebox/graphql"
	"github.com/texttheater/golang-levenshtein/levenshtein"
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
	log.Println("Bot is running...")
	client := graphql.NewClient("https://api.cofacts.tw/graphql")

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
					if len(msgtext) < 20 {
						break
					}

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
											type
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
					msgtext = strings.Replace(msgtext, "\"", "", -1)
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
											Type string
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
					if respData.ListArticles.TotalCount != 1 {
						// find out the most similar article
						var mostSimilarText string
						var minDistance int
						var target int

						for i := 0; i < respData.ListArticles.TotalCount; i++ {
							distance := levenshtein.DistanceForStrings([]rune(msgtext), []rune(respData.ListArticles.Edges[i].Node.Text), levenshtein.DefaultOptions)
							if minDistance == 0 || distance < minDistance {
								minDistance = distance
								mostSimilarText = respData.ListArticles.Edges[i].Node.Text
								target = i
							}
						}

						// do something with the most similar article
						fmt.Printf("The most similar article to '%s' is '%s'\n", msgtext, mostSimilarText)
						respData.ListArticles.Edges[0] = respData.ListArticles.Edges[target]

					}

					if respData.ListArticles.TotalCount != 0 && respData.ListArticles.Edges[0].Node.ArticleReplies[0].Reply.Type == "RUMOR" {
						log.Print("檢測可能為不實資訊，系統產生報告中...")
						data := (`
						{
							"model": "gpt-3.5-turbo",
							"max_tokens": 2000,
							"messages": [
								{
									"role": "system",
									"content": "請幫我使用繁體中文，以第三者的角度去客觀的解釋這個問題，請避開政治議題，並在最後給出結論"
								},
								{
									"role": "user",
									"content": "<replace>"
								}
							]
						}
						`)
						artext := respData.ListArticles.Edges[0].Node.ArticleReplies[0].Reply.Text
						artext = strings.Replace(artext, "\n", "", -1)
						artext = strings.Replace(artext, "\"", "", -1)
						message := "謠言:" + msgtext + "。" + "解釋:" + artext
						log.Print(message)
						data = strings.Replace(data, "<replace>", message, 1)
						req, err := http.NewRequest("POST", "https://api.pawan.krd/v1/chat/completions", bytes.NewBuffer([]byte(data)))
						if err != nil {
							log.Fatal(err)
						}
						req.Header.Add("Authorization", "Bearer "+os.Getenv("PAWAN_TOKEN"))
						req.Header.Add("Content-Type", "application/json")
						client := &http.Client{}
						resp, err := client.Do(req)
						if err != nil {
							log.Fatal(err)
						}
						defer resp.Body.Close()

						type bodys struct {
							ID      string `json:"id"`
							Created int64  `json:"created"`
							Model   string `json:"model"`
							Object  string `json:"object"`
							Choices []struct {
								FinishReason string `json:"finish_reason"`
								Index        int    `json:"index"`
								Message      struct {
									Content string `json:"content"`
									Role    string `json:"role"`
								} `json:"message"`
							} `json:"choices"`
							Usage struct {
								PromptTokens     int `json:"prompt_tokens"`
								CompletionTokens int `json:"completion_tokens"`
								TotalTokens      int `json:"total_tokens"`
							} `json:"usage"`
						}

						body := bodys{}
						err = json.NewDecoder(resp.Body).Decode(&body)
						if err != nil {
							log.Fatal(err)
						}
						log.Println(body)
						if len(body.Choices) == 0 {
							log.Print("API error")
							if _, err = bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage("此訊息為謠言，但系統無法生成回復")).Do(); err != nil {
								log.Print(err)
							}
						} else {
							if _, err = bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage("偵測到謠言，AI回復僅供參考:"+body.Choices[0].Message.Content)).Do(); err != nil {
								log.Print(err)
							}
						}

					}
				default:
					log.Printf("not text message: %v", message)
				}

			}
		}
	})
	if err := http.ListenAndServe(":"+os.Getenv("PORT"), nil); err != nil {
		log.Fatal(err)
	}
}
