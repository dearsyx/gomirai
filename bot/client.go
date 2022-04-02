package bot

import (
	"errors"
	"io"
	"strconv"
	"time"

	"gopkg.in/h2non/gentleman.v2/plugins/multipart"

	"gopkg.in/h2non/gentleman.v2"
	"gopkg.in/h2non/gentleman.v2/plugins/body"

	"github.com/sirupsen/logrus"

	"github.com/Logiase/gomirai/tools"
)

// Client 与Mirai进行沟通
type Client struct {
	Name    string
	AuthKey string

	HttpClient *gentleman.Client
	Bots       map[uint]*Bot

	Logger *logrus.Entry
}

// NewClient 新建Client
func NewClient(name, url, authKey string) *Client {
	c := gentleman.New()
	c.URL(url)

	return &Client{
		AuthKey:    authKey,
		HttpClient: c,
		Bots:       make(map[uint]*Bot),
		Logger: logrus.New().WithFields(logrus.Fields{
			"client": name,
		}),
	}
}

// --- API-HTTP插件相关 ---

// About 使用此方法获取插件的信息，如版本号
func (c *Client) About() (string, error) {
	res, err := c.doGet("/about", nil)
	if err != nil {
		return "", err
	}
	return tools.Json.Get([]byte(res), "data").Get("version").ToString(), nil
}

// --- 认证相关 ---

// Verify 使用此方法验证你的身份，并返回一个会话
func (c *Client) Verify() (string, error) {
	data := map[string]string{"verifyKey": c.AuthKey}
	res, err := c.doPost("/verify", data)
	if err != nil {
		return "", err
	}
	c.Logger.Info("Authed")
	return tools.Json.Get([]byte(res), "session").ToString(), nil
}

// Bind 使用此方法校验并激活你的Session，同时将Session与一个已登录的Bot绑定
func (c *Client) Bind(qq uint, sessionKey string) (*Bot, error) {
	data := map[string]interface{}{"sessionKey": sessionKey, "qq": qq}
	_, err := c.doPost("/bind", data)
	if err != nil {
		return nil, err
	}
	c.Bots[qq] = &Bot{QQ: qq, SessionKey: sessionKey, Client: c, Logger: c.Logger.WithField("qq", qq)}
	c.Bots[qq].SetChannel(time.Second, 10)
	c.Logger.Info("Verified")
	return c.Bots[qq], nil
}

// Release 使用此方式释放session及其相关资源（Bot不会被释放）
// 不使用的Session应当被释放，长时间（30分钟）未使用的Session将自动释放，否则Session持续保存Bot收到的消息，将会导致内存泄露(开启websocket后将不会自动释放)
func (c *Client) Release(qq uint) error {
	data := map[string]interface{}{"sessionKey": c.Bots[qq].SessionKey, "qq": qq}
	_, err := c.doPost("release", data)
	if err != nil {
		return err
	}
	delete(c.Bots, qq)
	c.Logger.Info("Released")
	return nil
}

// --- internal ---

func (c *Client) doPost(path string, data interface{}) (string, error) {
	c.Logger.Trace("POST:"+path+" Data:", data)
	res, err := c.HttpClient.Request().
		Path(path).
		Method("POST").
		Use(body.JSON(data)).
		SetHeader("Content-Type", "application/json;charset=utf-8").
		Send()
	if err != nil {
		c.Logger.Warn("POST Failed")
		return "", err
	}
	c.Logger.Trace("result StatusCode:", res.StatusCode)
	if !res.Ok {
		return res.String(), errors.New("Http: " + strconv.Itoa(res.StatusCode))
	}
	if tools.Json.Get([]byte(res.String()), "code").ToInt() != 0 {
		return res.String(), getErrByCode(tools.Json.Get([]byte(res.String()), "code").ToUint())
	}
	return res.String(), nil
}

func (c *Client) doPostWithFormData(path string, fields map[string]interface{}) (string, error) {
	data := make(multipart.DataFields)
	files := make([]multipart.FormFile, 0)

	for key, value := range fields {
		if unbox, ok := value.(string); ok {
			data[key] = append(data[key], unbox)
		} else if unbox, ok := value.(io.Reader); ok {
			files = append(files, multipart.FormFile{Name: key, Reader: unbox})
		}
	}

	formData := multipart.FormData{Data: data, Files: files}

	c.Logger.Trace("POST:"+path+" FormData:", formData)
	res, err := c.HttpClient.Request().
		Path(path).
		Method("POST").
		Use(multipart.Data(formData)).
		Send()
	if err != nil {
		c.Logger.Warn("POST Failed")
		return "", err
	}
	c.Logger.Trace("result StatusCode:", res.StatusCode)
	if !res.Ok {
		return res.String(), errors.New("Http: " + strconv.Itoa(res.StatusCode))
	}
	if tools.Json.Get([]byte(res.String()), "code").ToInt() != 0 {
		return res.String(), getErrByCode(tools.Json.Get([]byte(res.String()), "code").ToUint())
	}
	return res.String(), nil
}

func (c *Client) doGet(path string, params map[string]string) (string, error) {
	c.Logger.Trace("GET:" + path)
	res, err := c.HttpClient.Request().
		Path(path).
		SetQueryParams(params).
		Method("GET").
		SetHeader("Content-Type", "application/json;charset=utf-8").
		Send()
	if err != nil {
		c.Logger.Warn("GET Failed")
		return "", err
	}
	c.Logger.Trace("result StatusCode:", res.StatusCode)
	if !res.Ok {
		return res.String(), errors.New("Http: " + strconv.Itoa(res.StatusCode))
	}
	if tools.Json.Get([]byte(res.String()), "code").ToInt() != 0 {
		return res.String(), getErrByCode(tools.Json.Get([]byte(res.String()), "code").ToUint())
	}
	return res.String(), nil
}

func getErrByCode(code uint) error {
	switch code {
	case 0:
		return nil
	case 1:
		return errors.New("错误的auth key")
	case 2:
		return errors.New("指定的Bot不存在")
	case 3:
		return errors.New("Session失效或不存在")
	case 4:
		return errors.New("Session未认证(未激活)")
	case 5:
		return errors.New("发送消息目标不存在(指定对象不存在)")
	case 6:
		return errors.New("指定文件不存在，出现于发送本地图片")
	case 10:
		return errors.New("无操作权限，指Bot没有对应操作的限权")
	case 20:
		return errors.New("Bot被禁言，指Bot当前无法向指定群发送消息")
	case 30:
		return errors.New("消息过长")
	case 400:
		return errors.New("错误的访问，如参数错误等")
	default:
		return errors.New("未知错误,Code:" + string(code))
	}
}
