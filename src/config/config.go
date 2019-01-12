package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
)

var (
	EnvList         string //逗号分隔
	MsgKey          string
	MsgTitle        string
	AlertUrl        string //lark robot api
	RedisConnStr    string
	RedisPasswd     string
	RedisDB         int
	SimilarityValue uint8 //相似度值
	AlertDuration   int64 //相似的错误在此时间内(s)不会重复
	requiredEnv     = []string{"MSG_KEY", "MSG_TITLE", "ALERT_URL", "REDIS_CONN_STR", "REDIS_PASSWD", "REDIS_DB", "SIMILARITY_VALUE", "ALERT_DURATION"}

	K8s_tpl string
	Ecs_tpl string
	Regexp  *regexp.Regexp
)

type AlertBox struct {
	QA, UAT, PROD map[uint64]int64
}

func init() {
	//初始化消息模板、正则
	K8s_tpl = "时间: %s\n环境: %s\n名称空间: %s\n服务: %s\n容器: %s\n节点: %s\n内容:\n%s"
	Ecs_tpl = "时间: %s\n环境: %s\n服务: %s\n节点: %s\n内容:\n%s"
	reg_str := `(?P<time>^\w{3}\s+\d{1,2}\s\d{1,2}:\d{1,2}:\d{1,2})\s+(?P<hostname>\S+?)\s+(?P<program>\S+?):` //从syslog中捕获时间、主机、应用名称
	Regexp = regexp.MustCompile(reg_str)

	//获取环境变量
	for _, v := range requiredEnv {
		value := os.Getenv(v)
		if value == "" {
			if v != "REDIS_PASSWD" {
				panic(fmt.Sprintf("Fatal error: Env %s is requreid, exit...", v))
			}
		}

		switch v {
		case "MSG_KEY":
			MsgKey = value
		case "MSG_TITLE":
			MsgTitle = value
		case "ALERT_URL":
			AlertUrl = value
		case "REDIS_CONN_STR":
			RedisConnStr = value
		case "REDIS_PASSWD":
			RedisPasswd = value
		case "REDIS_DB":
			port, err := strconv.Atoi(value)
			if err != nil {
				panic(fmt.Sprintf("Fatal error: %s can't be converted to int", value))
			}
			RedisDB = port
		case "SIMILARITY_VALUE":
			t, err := strconv.ParseUint(value, 10, 8)
			if err != nil {
				panic(fmt.Sprintf("Fatal error: %s can't be converted to int", value))
			}
			SimilarityValue = uint8(t)
		case "ALERT_DURATION":
			t, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				panic(fmt.Sprintf("Fatal error: %s can't be converted to int", value))
			}
			AlertDuration = t
		}
	}
}
