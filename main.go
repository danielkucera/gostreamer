package main

import "github.com/gin-gonic/gin"
import "encoding/base64"

type Stream struct {
	Url	string
}

type Server struct {
	Streams map[string]*Stream
}

func (s *Server) createStream(Url string) *Stream {
	strm := &Stream {
		Url: Url,
	}
	s.Streams[Url] = strm
	return strm
}

func (s *Server) getStream(Url string) *Stream {
	if val, ok := s.Streams[Url]; ok {
		return val;
	} else {
		return s.createStream(Url)
	}
}


func main() {
	router := gin.Default()
	server := Server{
		Streams: make(map[string]*Stream, 0),
	}

	// This handler will match /user/john but will not match neither /user/ or /user
	router.GET("/stream/:id", func(c *gin.Context) {
		id := c.Param("id")
		Url, err := base64.StdEncoding.DecodeString(id)
		if err != nil {
			c.String(500, "Unable to decode stream id %s", id)
			return
		}

		strm := server.getStream(string(Url))
		if strm == nil {
			c.String(200, "Unable to start stream %s", Url)
		} else {
		}
	})

	// However, this one will match /user/john/ and also /user/john/send
	// If no other routers match /user/john, it will redirect to /user/john/
	router.GET("/status", func(c *gin.Context) {
		c.JSON(200, server)
	})

	router.Run(":8080")
}
