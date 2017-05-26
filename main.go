package main

import (
	"github.com/gin-gonic/gin"
	"encoding/base64"
	"encoding/csv"
	"os/exec"
	"io"
	"os"
	"log"
)

type Stream struct {
	Url	string
	Cmd	*exec.Cmd
	Stderr	string
	Stdout	io.ReadCloser
}

type Server struct {
	Streams map[string]*Stream
	Sources map[string]string
}

func (s *Server) createStream(Url string) *Stream {
	cmd := exec.Command("ffmpeg", "-i", Url, "-f", "mpegts", "-")

	strm := &Stream {
		Url: Url,
		Cmd: cmd,
	}
	s.Streams[Url] = strm

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("%s", err)
		return nil
	}
	strm.Stdout = stdout

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("%s", err)
		return nil
	}

	buf := make([]byte, 1024)
	done := make(chan bool)

	go func() {
		var err error
		var lng int
		for err == nil {
			lng, err = stderr.Read(buf)
			strm.Stderr = strm.Stderr + string(buf)[0:lng]
		}
		done <- true
	}()

	err = cmd.Start();
	if err != nil {
		log.Printf("%s", err)
		return nil
	}

	return strm
}

func (s *Server) getStream(Url string) *Stream {
	if val, ok := s.Streams[Url]; ok {
		return val;
	} else {
		return s.createStream(Url)
	}
}

func load_sources_csv(file string, server *Server){
	f, err := os.Open(file)
	if err != nil {
		panic(err)
	}

	r := csv.NewReader(f)

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		if len(record) > 1 {
			server.Sources[record[0]] = record[1]
		}

	}
}

func main() {
	router := gin.Default()

	server := Server{
		Streams: make(map[string]*Stream, 0),
		Sources: make(map[string]string, 0),
	}

	load_sources_csv("sources.csv", &server)

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

	router.GET("/", func(c *gin.Context) {
		page := ""
		for key, val := range server.Sources {
			id := base64.StdEncoding.EncodeToString([]byte(val))
			page = page + "<a href='/stream/"+ id +"'>" + key + "</a><br>\n"
		}
		c.Data(200, "text/html", []byte(page))
	})

	router.Run(":8080")
}
