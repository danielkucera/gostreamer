package main

import (
	"github.com/gin-gonic/gin"
	"encoding/base64"
	"encoding/csv"
	"os/exec"
	"bufio"
	"io"
	"os"
	"log"
	"time"
)

type Stream struct {
	Url	string
	Cmd	*exec.Cmd
	LastWrite time.Time
	Stderr	string
	Stdout	io.ReadCloser
	Clients map[*bufio.ReadWriter]bool `json:"-"`
//	Clients []*gin.Context `json:"-"`
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
		Clients: make(map[*bufio.ReadWriter]bool, 0),
	}
	s.Streams[Url] = strm

	log.Printf("starting stream %s", strm.Url)

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

	done := make(chan bool)

	go func() {
		buf := make([]byte, 1024)
		var err error
		var lng int
		for err == nil {
			lng, err = stderr.Read(buf)
			strm.Stderr = strm.Stderr + string(buf)[0:lng]
		}
		done <- true
	}()

	go func() {
		buf := make([]byte, 1024)
		var rerr error
		var lng int
		for rerr == nil {
			lng, rerr = stdout.Read(buf)
//			log.Printf("stdout read")
			for client, _ := range strm.Clients {
				strm.LastWrite = time.Now()
//				log.Printf("client write")
				_, err := client.Writer.Write(buf[0:lng])
				if err != nil {
					log.Printf("error writing to client: %s", err)
					delete(strm.Clients, client)
				}
			}
			if time.Since(strm.LastWrite) > 30*time.Second {
				log.Printf("no client on stream %s", strm.Url)
				break
			}
		}
		server.stopStream(strm.Url)
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

func (s *Server) stopStream(Url string) {
	log.Printf("stopping stream %s", Url)
	if val, ok := s.Streams[Url]; ok {
		delete(s.Streams, Url)
		val.Cmd.Process.Kill()
		//TODO: kill clients
	}
}

func (s *Stream) addClient(c *gin.Context) {
//	c.Data(200, "applicatiom/octet-stream", []byte("\n"))
//	c.Header("Content-Type", "application/octet-stream")
	headers := `HTTP/1.1 200 OK
Content-Type: applicatiom/octet-stream

`
	_, rw, _ := c.Writer.Hijack()
	rw.Writer.Write([]byte(headers))
	s.Clients[rw] = true
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

var server Server

func main() {
	router := gin.Default()

	server = Server{
		Streams: make(map[string]*Stream, 0),
		Sources: make(map[string]string, 0),
	}

	load_sources_csv("sources.csv", &server)

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
			strm.addClient(c)
		}
	})

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
