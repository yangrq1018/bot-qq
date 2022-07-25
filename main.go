package main

import (
	"log"
	"os"
	"os/signal"

	"github.com/Logiase/MiraiGo-Template/bot"
	"github.com/Logiase/MiraiGo-Template/config"
	_ "github.com/Logiase/MiraiGo-Template/modules/logging"
	_ "github.com/yangrq1018/botqq/modules"
)

func init() {
	config.Init()
}

func main() {
	// 快速初始化
	bot.Init()
	config.GlobalConfig.WatchConfig()

	// 初始化 Modules
	bot.StartService()

	// 使用协议
	// 不同协议可能会有部分功能无法使用
	// 在登陆前切换协议
	bot.UseProtocol(bot.AndroidPhone)

	// 登录
	err := bot.Login()
	if err != nil {
		log.Fatal(err)
	}

	// 刷新好友列表，群列表
	bot.RefreshList()

	// 保存session文件
	if config.GlobalConfig.GetBool("save_token") {
		bot.SaveToken()
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
	bot.Stop()
}
