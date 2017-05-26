package main

import "github.com/gin-gonic/gin"

type Stream struct {
	URL	string
}

var streams map[string]*Stream = make(map[string]*Stream, 0)

func main() {
	router := gin.Default()

	// This handler will match /user/john but will not match neither /user/ or /user
	router.GET("/stream/:name", func(c *gin.Context) {
		name := c.Param("name")
		c.String(200, "Hello %s", name)
	})

	// However, this one will match /user/john/ and also /user/john/send
	// If no other routers match /user/john, it will redirect to /user/john/
	router.GET("/status", func(c *gin.Context) {
		c.JSON(200, streams)
	})

	router.Run(":8080")
}
