package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2/utils"

	"github.com/finb/bark-server/v2/apns"

	"github.com/gofiber/fiber/v2"
)

func init() {
	// V2 API
	registerRoute("push", func(router fiber.Router) {
		router.Post("/push", func(c *fiber.Ctx) error { return routeDoPush(c) })
	})

	// compatible with old requests
	registerRouteWithWeight("push_compat", 1, func(router fiber.Router) {
		router.Get("/:device_key", func(c *fiber.Ctx) error { return routeDoPush(c) })
		router.Post("/:device_key", func(c *fiber.Ctx) error { return routeDoPush(c) })

		router.Get("/:device_key/:body", func(c *fiber.Ctx) error { return routeDoPush(c) })
		router.Post("/:device_key/:body", func(c *fiber.Ctx) error { return routeDoPush(c) })

		router.Get("/:device_key/:title/:body", func(c *fiber.Ctx) error { return routeDoPush(c) })
		router.Post("/:device_key/:title/:body", func(c *fiber.Ctx) error { return routeDoPush(c) })

		router.Get("/:device_key/:category/:title/:body", func(c *fiber.Ctx) error { return routeDoPush(c) })
		router.Post("/:device_key/:category/:title/:body", func(c *fiber.Ctx) error { return routeDoPush(c) })
	})
}

func routeDoPush(c *fiber.Ctx) error {
	if !canPush(c) {
		return c.JSON(success())
	}

	// Get content-type
	contentType := utils.ToLower(utils.UnsafeString(c.Request().Header.ContentType()))
	contentType = utils.ParseVendorSpecificContentType(contentType)
	// Json request uses the API V2
	if strings.HasPrefix(contentType, "application/json") {
		return routeDoPushV2(c)
	}

	params := make(map[string]interface{})
	visitor := func(key, value []byte) {
		params[strings.ToLower(string(key))] = string(value)
	}
	// parse query args (medium priority)
	c.Request().URI().QueryArgs().VisitAll(visitor)
	// parse post args
	c.Request().PostArgs().VisitAll(visitor)

	// parse multipartForm values
	form, err := c.Request().MultipartForm()
	if err == nil {
		for key, val := range form.Value {
			if len(val) > 0 {
				params[key] = val[0]
			}
		}
	}

	return push(c, params)
}

var whiteList = []string{"青龙客户端", "登录通知", "过期", "cookie已失效"}

func in(str string, s []string) bool {

	for _, v := range s {
		if strings.Contains(strings.ToLower(str), strings.ToLower(v)) {
			return true
		}
	}

	return false
}

func canPush(c *fiber.Ctx) bool {

	var title, _ = url.QueryUnescape(c.Params("title"))

	if title != "" && len(title) > 0 {
		var result = in(title, whiteList)
		if result {
			return true
		}
	}

	var desc, _ = url.QueryUnescape(c.Params("body"))
	if desc != "" && len(desc) > 0 {
		var result2 = in(desc, whiteList)
		if result2 {
			return true
		}
	}
	println(title)
	fmt.Println("not in whiteList, drop this push")

	return false
}

func routeDoPushV2(c *fiber.Ctx) error {

	params := make(map[string]interface{})
	// parse body
	if err := c.BodyParser(&params); err != nil && err != fiber.ErrUnprocessableEntity {
		return c.Status(400).JSON(failed(400, "request bind failed: %v", err))
	}
	// parse query args (medium priority)
	c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
		params[strings.ToLower(string(key))] = string(value)
	})
	return push(c, params)
}

func push(c *fiber.Ctx, params map[string]interface{}) error {

	// default value
	msg := apns.PushMessage{
		Category:  "myNotificationCategory",
		Body:      "NoContent",
		Sound:     "1107",
		ExtParams: make(map[string]interface{}),
	}

	for key, val := range params {
		switch val := val.(type) {
		case string:
			switch strings.ToLower(string(key)) {
			case "device_key":
				msg.DeviceKey = val
			case "category":
				msg.Category = val
			case "title":
				msg.Title = val
			case "body":
				msg.Body = val
			case "sound":
				// Compatible with old parameters
				if strings.HasSuffix(val, ".caf") {
					msg.Sound = val
				} else {
					msg.Sound = val + ".caf"
				}
			default:
				msg.ExtParams[strings.ToLower(string(key))] = val
			}
		case map[string]interface{}:
			for k, v := range val {
				msg.ExtParams[k] = v
			}
		default:
			msg.ExtParams[key] = val
		}
	}

	// parse url path (highest priority)
	if pathDeviceKey := c.Params("device_key"); pathDeviceKey != "" {
		msg.DeviceKey = pathDeviceKey
	}
	if category := c.Params("category"); category != "" {
		str, err := url.QueryUnescape(category)
		if err != nil {
			return err
		}
		msg.Category = str
	}
	if title := c.Params("title"); title != "" {
		str, err := url.QueryUnescape(title)
		if err != nil {
			return err
		}
		msg.Title = str
	}
	if body := c.Params("body"); body != "" {
		str, err := url.QueryUnescape(body)
		if err != nil {
			return err
		}
		msg.Body = str
	}

	if msg.DeviceKey == "" {
		return c.Status(400).JSON(failed(400, "device key is empty"))
	}

	deviceToken, err := db.DeviceTokenByKey(msg.DeviceKey)
	if err != nil {
		return c.Status(400).JSON(failed(400, "failed to get device token: %v", err))
	}
	msg.DeviceToken = deviceToken

	err = apns.Push(&msg)
	if err != nil {
		return c.Status(500).JSON(failed(500, "push failed: %v", err))
	}
	return c.JSON(success())
}
