package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	_ "io/ioutil" //调试时使用
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"Alert/src/config"

	"github.com/go-redis/redis"
	"github.com/mfonda/simhash"
)

func main() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals) //注意kill -9是不能捕获的

	//连接redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     config.RedisConnStr,
		Password: config.RedisPasswd,
		DB:       config.RedisDB,
	})
	defer redisClient.Close()

	msgChan := make(chan *KafkaMessage, 5)

	//创建SafeAlertMap, 并初始化内部的map
	AlertMap := &SafeAlertMap{alertMap: make(map[uint64]int64)}

	//从Redis的console-err中获取日志，并放入msgChan通道中
	go fetchMsgFromRedis(redisClient, msgChan, AlertMap)

	//将取出的消息通过渠道发出去
	go sendMsg(msgChan, config.AlertUrl)

	//监听退出信号
	for {
		select {
		case sig := <-signals:
			log.Printf("收到 %v 信号，主程序退出...\n", sig)
			return
		}
	}

}

type LarkRobotMsg struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

//map 不是线程安全的
type SafeAlertMap struct {
	sync.RWMutex
	alertMap map[uint64]int64 //key: simhash_value, value: current_timestamp
}

//filebeat发往kafka的消息结构，其中不需要的字段都使用空接口来接收，在反序列化的时候会自动的对其进行转换
type KafkaMessage struct {
	Timestamp  string            `json:"@timestamp"`
	Metadata   interface{}       `json:"@metadata"`
	Source     interface{}       `json:"source"`
	Fields     map[string]string `json:"fields"`
	Host       map[string]string `json:"host"`
	Offset     int               `json:"offset"`
	Stream     interface{}       `json:"stream"`
	Message    string            `json:"message"`
	Prospector interface{}       `json:"prospector"`
	Input      interface{}       `json:"input"`
	Kubernetes K8sFileds         `json:"kubernetes"`
	Beat       interface{}       `json:"beat"`
}

type K8sFileds struct {
	NameSpace  string            `json:"namespace"`
	Container  map[string]string `json:"container"`
	Replicaset map[string]string `json:"replicaset"`
	Labels     map[string]string `json:"labels"`
	Pod        map[string]string `json:"pod"`
	Node       map[string]string `json:"node"`
}

func fetchMsgFromRedis(client *redis.Client, msgChan chan<- *KafkaMessage, AlertMap *SafeAlertMap) {
	for {
		msg, err := client.BRPop(time.Second, config.MsgKey).Result()
		if err != nil {
			// log.Println("从Redis获取消息失败，err = ", err)
		} else {
			for _, v := range msg[1:] { //msg的第一个元素是list的名称，故弹出
				go check(AlertMap, v, msgChan) //并发访问MAP，可能会出现问题。
			}
		}
	}
}

func check(AlertMap *SafeAlertMap, v string, msgChan chan<- *KafkaMessage) {
	//反序列化日志
	logEntry := &KafkaMessage{}
	err := json.Unmarshal([]byte(v), logEntry)
	if err != nil {
		log.Println("反序列化消息失败, 跳过该消息")
		return
	}

	msg := logEntry.Message

	hash := simhash.Simhash(simhash.NewWordFeatureSet([]byte(msg)))
	timestamp := time.Now().Unix()
	needAlert := true
	//检查当前hash在不在AlertMap中
	AlertMap.Lock() //检查前加锁
	_, ok := AlertMap.alertMap[hash]
	if !ok {
		//未查找到，计算当前hash和AlertMap中的所有hash的相似度，如果小于等于 SimlarityValue 则说明该消息已经发送过。
		for key, value := range AlertMap.alertMap {
			simValue := simhash.Compare(hash, key)
			if simValue <= config.SimilarityValue {
				if timestamp-value > config.AlertDuration {
					//重复的日志过了一小时后需要重新发
					msgChan <- logEntry
					//发完后需要更新AlertMap
					AlertMap.alertMap[key] = timestamp
				}
				needAlert = false
				break
			}
		}
		if needAlert {
			//将hash, timestamp 添加到 AlertMap中
			AlertMap.alertMap[hash] = timestamp

			//将消息发出去
			msgChan <- logEntry
		}
	}
	//释放锁
	AlertMap.Unlock()
}

func sendMsg(msgChan <-chan *KafkaMessage, sendUrl string) {
	for {
		select {
		case msg := <-msgChan:
			var tpl string
			if _, ok := msg.Kubernetes.Container["name"]; ok {
				tpl = fmt.Sprintf(config.K8s_tpl, msg.Timestamp, msg.Fields["env"], msg.Kubernetes.NameSpace, msg.Kubernetes.Labels["app"], msg.Kubernetes.Pod["name"],
					msg.Kubernetes.Node["name"], msg.Message)
			} else {
				groups := config.Regexp.FindStringSubmatch(msg.Message)
				tpl = fmt.Sprintf(config.Ecs_tpl, msg.Timestamp, msg.Fields["env"], groups[3], groups[2], msg.Message)
			}

			larkMsg := LarkRobotMsg{Title: config.MsgTitle, Text: tpl}
			jLarkMsg, err := json.Marshal(larkMsg)
			if err != nil {
				log.Println("反序列化日志失败，源始日志格式必须为kafka消息。err = ", err)
			} else {
				body := bytes.NewReader(jLarkMsg)
				tr := &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				}
				client := &http.Client{Timeout: 5 * time.Second, Transport: tr}
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
