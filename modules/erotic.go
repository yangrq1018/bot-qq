package modules

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/Logiase/MiraiGo-Template/bot"
	"github.com/Logiase/MiraiGo-Template/config"
	"github.com/Mrs4s/MiraiGo/client"
	"github.com/Mrs4s/MiraiGo/message"
)

var instanceErotic *erotic

const (
	setuCommand = "/erotic"
)

// pixiv 图片需要代理下载
var proxiedClient = http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	},
}

type erotic struct {
	base
	loliconURL string
}

func (s erotic) MiraiGoModule() bot.ModuleInfo {
	return bot.ModuleInfo{
		ID:       "setu",
		Instance: instanceErotic,
	}
}

func (s *erotic) Init() {
	s.base.Init()
	s.loliconURL = config.GlobalConfig.GetString("modules.erotic.url")
}

func (s erotic) PostInit() {}

func (s *erotic) Serve(bot *bot.Bot) {
	s.monitorGroups.Each(func(code int64) {
		registerMessageListener(code, s.dispatch, &bot.GroupMessageEvent, &bot.SelfGroupMessageEvent)
	})
}

func (s erotic) Start(_ *bot.Bot) {}

func (s erotic) Stop(_ *bot.Bot, wg *sync.WaitGroup) {
	defer wg.Done()
}

func (s *erotic) dispatch(client *client.QQClient, msg *message.GroupMessage) {
	if s.isBotCommand(msg) {
		if text := textMessage(msg); text != nil {
			cmd, _ := command(text)
			switch cmd {
			case setuCommand:
				go func() {
					if err := s.handleCmd(client, msg); err != nil {
						logger.Errorf("%s handle error: %s", setuCommand, err)
					}
				}()
			}
		}
	}
}

func (s *erotic) handleCmd(client *client.QQClient, msg *message.GroupMessage) error {
	res, err := proxiedClient.Get(s.loliconURL)
	if err != nil {
		return err
	}
	var data struct {
		Error string `json:"error"`
		Data  []struct {
			Pid        int      `json:"pid"`
			P          int      `json:"p"`
			Uid        int      `json:"uid"`
			Title      string   `json:"title"`
			Author     string   `json:"author"`
			R18        bool     `json:"r18"`
			Width      int      `json:"width"`
			Height     int      `json:"height"`
			Tags       []string `json:"tags"`
			Ext        string   `json:"ext"`
			UploadDate int64    `json:"uploadDate"`
			Urls       struct {
				Original string `json:"original"`
			} `json:"urls"`
		} `json:"data"`
	}
	err = json.NewDecoder(res.Body).Decode(&data)
	_ = res.Body.Close()
	if err != nil {
		return err
	}
	if len(data.Data) == 0 {
		return fmt.Errorf("no data: sepi gun")
	}
	image, err := readImageURI(data.Data[0].Urls.Original)
	if err != nil {
		return err
	}
	msg2 := pictureMessage(client, msg.GroupCode, image).Append(message.NewText("一张涩图"))
	client.SendGroupMessage(msg.GroupCode, msg2)
	return nil
}
