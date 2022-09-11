package modules

import (
	"os"

	"github.com/yangrq1018/botqq/mongodb"

	"github.com/Logiase/MiraiGo-Template/bot"
	"github.com/Logiase/MiraiGo-Template/config"
	"github.com/Logiase/MiraiGo-Template/utils"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	logger      = utils.GetModuleLogger("qq")
	mongoClient *mongo.Client
)

func init() {
	// imported packages are initialized before the package itself,
	// so make sure that config.GlobalConfig is initialized before
	// reading it
	if config.GlobalConfig == nil {
		config.Init()
	}

	instanceRoll = new(roll)
	instanceErotic = new(erotic)
	instanceManage = new(manage)

	bot.RegisterModule(instanceRoll)
	bot.RegisterModule(instanceErotic)
	bot.RegisterModule(instanceManage)

	_mongoClient, err := mongodb.NewClient(os.Getenv("MONGO_URI"), os.Getenv("MONGO_PROXY"))
	if err != nil {
		logger.Fatalf("failed to create mongo client: %v", err)
	}
	mongoClient = _mongoClient
}
