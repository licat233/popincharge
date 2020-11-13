
package main

import (
	"compress/gzip"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	//数据库驱动
	_ "github.com/go-sql-driver/mysql"
)

func main() {
	for {
		a := Popin.GetYesterdayCharge()
		fmt.Println(time.Now().Format("2006-01-02 15:04:05"),"：",a)
		popinDB.ReplaceYesterdayCharge(a)
		<-time.NewTimer(time.Hour * 6).C
	}
}

//SQL 数据库结构体
type SQL struct {
	Addr     string `json:"mysql_addr"`
	Account  string `json:"mysql_account"`
	Password string `json:"mysql_password"`
	DBname   string `json:"mysql_dbname"`
	TBname   string `json:"mysql_tbname"`
	Conn     *sql.DB
}

//POP POPin的相关信息
type POP struct {
	Account string `json:"popin_account"`
	Cookie  string `json:"popin_cookie"`
}

//Popin 实例化一个POP对象
var Popin POP

//Mysql 数据库对象
var popinDB SQL

func init() {
	var file *os.File
	var err error
	var configByte []byte
	if file, err = os.Open("config.json"); err != nil {
		log.Fatal(err)
	}
	if configByte, err = ioutil.ReadAll(file); err != nil {
		log.Fatal(err)
	}
	if err = json.Unmarshal(configByte, &Popin); err != nil {
		log.Fatal(err)
	}
	if err = json.Unmarshal(configByte, &popinDB); err != nil {
		log.Fatal(err)
	}

	popinDB.Conn, err = sql.Open("mysql", popinDB.Account+":"+popinDB.Password+"@tcp("+popinDB.Addr+")/"+popinDB.DBname)
	if err != nil {
		panic(err.Error())
	} else {
		fmt.Printf("已连接至数据库：%s\n", popinDB.Addr)
	}

	mainsql := "CREATE TABLE IF NOT EXISTS `" + popinDB.DBname + "`.`" + popinDB.TBname + "` ( `id` INT NOT NULL AUTO_INCREMENT ,`allday_charge` INT(64) DEFAULT 0 ,`allmonth_charge` INT(64) DEFAULT 0 , `date` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP , PRIMARY KEY (`id`)) ENGINE = InnoDB;"
// 	fmt.Println(mainsql)
	stmtIns, err := popinDB.Conn.Query(mainsql)

	if err != nil {
		panic(err.Error())
	}
	defer stmtIns.Close()
}

//ReplaceYesterdayCharge 更新昨天数据，不存在则创建，零点到零点时间段
func (t *SQL) ReplaceYesterdayCharge(Charge int) {
	datestr := GetYesterdaydate()
	sqlml := "REPLACE INTO " + t.TBname + " (id,allday_charge) VALUES('" + datestr + "','" + strconv.Itoa(Charge) + "');"
	stmtOut, err := t.Conn.Query(sqlml)
	if err != nil {
		fmt.Println(err)
	}
	defer stmtOut.Close()
}

//GetYesterdayCharge 爬取所需要的数据（消耗额）,核心函数,不要手贱的去"优化"
func (pop *POP) GetYesterdayCharge() int {
	thedate := GetYesterdaydate()
	targetURL := "https://dashboard.popin.cc/discovery/accounts-tw/index.php/" + pop.Account + "/campaigns/viewClientDashboard?start_date=" + thedate + "&stop_date=" + thedate
	referer := "https://dashboard.popin.cc/discovery/accounts-tw/index.php/" + pop.Account + "/manageAgency?subPage=/manageDashboard"

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		fmt.Println("http.NewRequest Failed:", err)
		return 0
	}

	req.Header.Set("Host", "dashboard.popin.cc")
	req.Header.Set("User-Agent", GetRandomUserAgent())
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", referer)
	req.Header.Set("Cookie", pop.Cookie)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("X-CSRF-TOKEN", "cu3UZOZgOIQeEP2EilP9W0mGKPz4I3kiaocGgmc6")
	//发送请求数据之服务器，并获取响应数据
	var resp *http.Response
	resp, err = client.Do(req)
	if err != nil {
		fmt.Println("Client Do Failed:", err)
		return 0
	}

	if resp.StatusCode != 200 {
		fmt.Println("status code error: ", resp.StatusCode, resp.Status)
		return 0
	}
	defer resp.Body.Close()

	var body string
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, _ := gzip.NewReader(resp.Body)
		for {
			buf := make([]byte, 1024)
			n, err := reader.Read(buf)

			if err != nil && err != io.EOF {
				panic(err)
			}

			if n == 0 {
				break
			}
			body += string(buf)
		}
	default:
		bodyByte, _ := ioutil.ReadAll(resp.Body)
		body = string(bodyByte)
	}

	r := regexp.MustCompile(`\d*(,?\d*)*元`)
	d := r.FindString(body)
	var y string = "元"
	d1 := strings.Replace(d, y, "", -1)
	if d1 == "" || d1 == "0" {
		return 0
	}
	d2 := strings.Replace(d1, ",", "", -1)
	d3, err := strconv.Atoi(d2)
	if err != nil {
		return 0
	}

	return d3
}

//GetYesterdaydate 常规---获取昨天日期
func GetYesterdaydate() string {
	now := time.Now()
	d, _ := time.ParseDuration("-24h")
	dres := now.Add(d)
	yesterday := dres.Format("20060102")
	return yesterday
}

//GetRandomUserAgent 从切片中随机抽取一个user-agent
func GetRandomUserAgent() string {
	var userAgentList = []string{"Mozilla/5.0 (compatible, MSIE 10.0, Windows NT, DigExt)",
		"Mozilla/4.0 (compatible, MSIE 8.0, Windows NT 6.0, Trident/4.0)",
		"Mozilla/4.0 (compatible, MSIE 7.0, Windows NT 5.1, 360SE)",
		"Mozilla/5.0 (compatible, MSIE 9.0, Windows NT 6.1, Trident/5.0,",
		"Opera/9.80 (Windows NT 6.1, U, en) Presto/2.8.131 Version/11.11",
		"Mozilla/4.0 (compatible, MSIE 7.0, Windows NT 5.1, TencentTraveler 4.0)",
		"Mozilla/5.0 (Windows, U, Windows NT 6.1, en-us) AppleWebKit/534.50 (KHTML, like Gecko) Version/5.1 Safari/534.50",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/79.0.3945.117 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:72.0) Gecko/20100101 Firefox/72.0",
		"Mozilla/5.0 (Macintosh, Intel Mac OS X 10_7_0) AppleWebKit/535.11 (KHTML, like Gecko) Chrome/17.0.963.56 Safari/535.11",
		"Mozilla/5.0 (Macintosh, U, Intel Mac OS X 10_6_8, en-us) AppleWebKit/534.50 (KHTML, like Gecko) Version/5.1 Safari/534.50",
		"Mozilla/5.0 (Linux, U, Android 3.0, en-us, Xoom Build/HRI39) AppleWebKit/534.13 (KHTML, like Gecko) Version/4.0 Safari/534.13",
		"Mozilla/5.0 (iPad, U, CPU OS 4_3_3 like Mac OS X, en-us) AppleWebKit/533.17.9 (KHTML, like Gecko) Version/5.0.2 Mobile/8J2 Safari/6533.18.5",
		"Mozilla/4.0 (compatible, MSIE 7.0, Windows NT 5.1, Trident/4.0, SE 2.X MetaSr 1.0, SE 2.X MetaSr 1.0, .NET CLR 2.0.50727, SE 2.X MetaSr 1.0)",
		"Mozilla/5.0 (iPhone, U, CPU iPhone OS 4_3_3 like Mac OS X, en-us) AppleWebKit/533.17.9 (KHTML, like Gecko) Version/5.0.2 Mobile/8J2 Safari/6533.18.5",
		"MQQBrowser/26 Mozilla/5.0 (Linux, U, Android 2.3.7, zh-cn, MB200 Build/GRJ22, CyanogenMod-7) AppleWebKit/533.1 (KHTML, like Gecko) Version/4.0 Mobile Safari/533.1"}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return userAgentList[r.Intn(len(userAgentList))]
}
