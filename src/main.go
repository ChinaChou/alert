package main

import (
	"time"
	"bytes"
	"log"
	"os"
	"os/signal"
	_ "io/ioutil"//调试时使用
	"net/http"
	"encoding/json"
	"github.com/go-redis/redis"
)

var (
	MsgKey			string = "console-err"
	MsgTitle		string = "Console error logs"
	SendUrl			string = "http://localhost:8080/sendMsg"
	RedisConnStr	string = "localhost:6379"
	RedisPasswd		string = ""
	RedisDB			int = 0
)


type LarkRobotMsg struct {
	Title	string	`json:"title"`
	Text	string	`json:"text"`
}


func fetchMsgFromRedis(client *redis.Client, msgChan chan<- string) {
	for {
		msg, err := client.BRPop(time.Second, MsgKey).Result()
		if err != nil {
			// log.Println("从Redis获取消息失败，err = ", err)
		} else {
			for _, v := range msg[1:] {	//msg的第一个元素是list的名称，故弹出
				// log.Println("msg = ", v)
				msgChan <- v
			}
		}
	}
}


func sendMsg(msgChan <-chan string, sendUrl string) {
	for {
		select {
		case msg := <-msgChan:
			larkMsg := LarkRobotMsg{Title: MsgTitle, Text: msg}
			jLarkMsg, err := json.Marshal(larkMsg)
			if err != nil {
				log.Println("反序列化日志失败，err = ", err)
			} else {
				body := bytes.NewReader(jLarkMsg)
				client := &http.Client{Timeout: 5 * time.Second}
				req, _ := http.NewRequest("POST", sendUrl, body)
				req.Header.Add("Content-Type", "application/json")
				resp, err := client.Do(req)
				if err != nil {
					log.Println("发送日志失败, err = ", err)
				} else {
					// respContent, _ := ioutil.ReadAll(resp.Body)
					// log.Println("发送的结果：", respContent)
					resp.Body.Close()
				}
			}	
		}
	}
	
}



func main() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals)	//注意kill -9是不能捕获的

	//连接redis
	redisClient := redis.NewClient(&redis.Options{
		Addr: RedisConnStr,
		Password: RedisPasswd,
		DB: RedisDB,
	})
	defer redisClient.Close()

	msgChan := make(chan string, 5)
	//从Redis的console-err中获取日志，并放入msgChan通道中
	go fetchMsgFromRedis(redisClient, msgChan)

	//将取出的消息通过渠道发出去
	go sendMsg(msgChan, SendUrl)

	for {
		select {
			case sig := <-signals:
				log.Printf("收到 %v 信号，主程序退出...\n", sig)
				return
		}
	}

}