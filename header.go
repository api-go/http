package http

const (
	HeaderClientIP         = "X-Real-IP"            // 真实的用户IP
	HeaderForwardedFor     = "X-Forwarded-For"      // 客户端IP列表
	HeaderUserID           = "X-User-ID"            // 用户编号
	HeaderDeviceID         = "X-Device-ID"          // 设备唯一编号
	HeaderClientAppName    = "X-Client-App-Name"    // 客户端App名字
	HeaderClientAppVersion = "X-Client-App-Version" // 客户端App版本号
	HeaderSessionID        = "X-Session-ID"         // 会话唯一编号
	HeaderRequestID        = "X-Request-ID"         // 请求唯一编号
	HeaderHost             = "X-Host"               // 真实用户请求的Host
	HeaderScheme           = "X-Scheme"             // 真实用户请求的 http or https
	HeaderFromApp          = "X-From-App"           // 来源App
	HeaderFromNode         = "X-From-Node"          // 来源节点
	HeaderUserAgent        = "X-User-Agent"         // 真实用户的UserAgent
)

// RelayHeaders 需要透传的头
var RelayHeaders = []string{
	HeaderUserID,
	HeaderDeviceID,
	HeaderClientAppName,
	HeaderClientAppVersion,
	HeaderSessionID,
	HeaderRequestID,
	HeaderHost,
	HeaderScheme,
	HeaderFromApp,
	HeaderFromNode,
	HeaderUserAgent,
}
