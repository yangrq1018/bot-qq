package main

import (
	"github.com/Logiase/MiraiGo-Template/bot"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestGenDevice(t *testing.T) {
	bot.UseProtocol(bot.AndroidPhone)
	bot.GenRandomDevice()
}

func TestParseDrawTime(t *testing.T) {
	_, err := time.ParseInLocation("2006-01-02 15:04", "2022-06-18 24:00", time.Local)
	assert.Error(t, err)

	_, err = time.ParseInLocation("2006-01-02 15:04", "2022-06-19 00:00", time.Local)
	assert.NoError(t, err)
}
