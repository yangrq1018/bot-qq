package modules

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestReadConfig(t *testing.T) {
	instanceManage.Init()
	assert.NotEmpty(t, instanceManage.embyURL)
	assert.NotEmpty(t, instanceManage.embyToken)
	assert.NotEmpty(t, instanceManage.sendTime)
	assert.NotEmpty(t, instanceManage.clearTime)
}
