package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// paymentReturnPath 计算支付成功或取消后需要跳转的前端路径。
// 参数：
//   - suffix：具体业务页面的相对路径后缀，例如充值结果页或订阅结果页。
//
// 返回：
//   - string：拼接站点地址和主题感知路径后的完整跳转地址。
func paymentReturnPath(suffix string) string {
	// 先去掉服务端地址末尾多余的斜杠，避免后续路径拼接出现双斜杠。
	base := strings.TrimRight(system_setting.ServerAddress, "/")

	// 再根据当前主题前缀拼出最终返回路径，兼容不同前端主题入口。
	return base + common.ThemeAwarePath(suffix)
}
