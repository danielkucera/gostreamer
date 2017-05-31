package main

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/gin-gonic/gin"
	"encoding/csv"
	"strconv"
	"os/exec"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"log"
	"time"
)

var db_file = "./db.sqlite"

type Stream struct {
	Id	string
	Url	string
	Active	bool
	Cmd	*exec.Cmd
	LastWrite time.Time
	Playlist string
	LastChunk string
	Stderr	string
	Stdout	io.ReadCloser
}

type Source struct {
	Id	int
	Name	string
	Url	string
	Weight	int
}

type Server struct {
	Streams map[string]*Stream
	DB	*sql.DB
}

func (s *Server) getSources() []*Source {
	srcs := make([]*Source,0)

        rows, err := s.DB.Query("SELECT `id`,`name`,`url`,`weight` FROM `sources` ORDER BY `weight` DESC, `id` ASC")
        checkErr(err)

        for rows.Next() {
            var src Source
            err = rows.Scan(&src.Id, &src.Name, &src.Url, &src.Weight)
            checkErr(err)
            srcs = append(srcs, &src)
        }

	return srcs
}

func (s *Server) addSource(src *Source) error {
	log.Printf("addding %s", src.Name)
	stmt, err := s.DB.Prepare("INSERT INTO `sources` (`name`, `url`, `weight`) VALUES (?,?,?)")
        if err != nil {
		return err
	}
	_, err = stmt.Exec(src.Name,src.Url,src.Weight)
        if err != nil {
		return err
	}
	return nil
}

func (s *Server) updateSource(src *Source) error {
	log.Printf("updating %s", src.Name)
	stmt, err := s.DB.Prepare("UPDATE `sources` SET `name` = ?, `url` = ?, `weight` = ? WHERE id = ?")
        if err != nil {
		return err
	}
	_, err = stmt.Exec(src.Name,src.Url,src.Weight,src.Id)
        if err != nil {
		return err
	}
	return nil
}

func (s *Server) deleteSource(src *Source) error {
	log.Printf("deleting %s", src.Name)
	stmt, err := s.DB.Prepare("DELETE FROM `sources` WHERE id = ?")
        if err != nil {
		return err
	}
	_, err = stmt.Exec(src.Id)
        if err != nil {
		return err
	}
	return nil
}

func (s *Server) getSourceById(id int) *Source {

        stmt, err := s.DB.Prepare("SELECT `id`,`name`,`url`,`weight` FROM `sources` WHERE `id` = ?")
        checkErr(err)

	rows, err := stmt.Query(id)
	checkErr(err)

        if rows.Next() {
            var src Source
            err = rows.Scan(&src.Id, &src.Name, &src.Url, &src.Weight)
            checkErr(err)
            return &src
        } else {
	    return nil
	}
}

func (s *Server) createStream(id string) *Stream {

	iid, _ := strconv.Atoi(id)

	strm := &Stream {
		Id: id,
		LastWrite: time.Now(),
		Url: server.getSourceById(iid).Url,
		Active: true,
	}
	s.Streams[id] = strm

	cmd := exec.Command(
		"./ffmpeg",
		"-i", strm.Url,
		"-map", "0",
		"-copy_unknown",
		"-sn", //subtitle none
		"-dn", //data none
		"-deinterlace",
		"-c:v", "h264",
		"-preset", "fast",
		"-b:v", "1024k",
		"-c:a", "aac",
		"-b:a", "192k",
		"-start_number", "0",
		"-hls_init_time", "2",
		"-hls_time", "6",
		"-hls_list_size", "10",
		"-use_localtime", "1",
		"-hls_base_url", "/data/",
		"-hls_segment_filename", "data/chunk-%Y%m%d-%s-stream-"+id+".ts",
		"-f", "hls",
		"-method", "PUT", "http://localhost:8080/stream/"+id+"/hls.m3u8",
	)

	strm.Cmd = cmd

	log.Printf("starting stream %s", strm.Url)

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
		for time.Since(strm.LastWrite) < 30*time.Second {
			time.Sleep(time.Second*5)
		}
		log.Printf("no client on stream %s", strm.Url)
		server.stopStream(id)
		done <- true
	}()

	err = cmd.Start();
	if err != nil {
		log.Printf("%s", err)
		return nil
	}

	return strm
}

func (s *Server) getStream(id string) *Stream {
	if val, ok := s.Streams[id]; ok && val.Active {
		return val;
	} else {
		return s.createStream(id)
	}
}

func (s *Server) stopStream(id string) {
	log.Printf("stopping stream %s", id)
	if val, ok := s.Streams[id]; ok {
		val.Active = false
		val.Cmd.Process.Kill()
		//TODO: kill clients
	}
}

func (s *Server) importSourcesCsv(f io.Reader) error {
	var src Source
	r := csv.NewReader(f)

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if len(record) > 1 {
			src.Name = record[0]
			src.Url = record[1]
			if len(record) > 2 {
				iid,_ := strconv.Atoi(record[2])
				src.Weight = iid

			}
			err = s.addSource(&src)
			if err != nil {
			        return err
			}
		}
	}
	return nil
}

func (s *Stream) serveClient(c *gin.Context) {
//	c.Data(200, "applicatiom/octet-stream", []byte("\n"))
//	c.Header("Content-Type", "application/octet-stream")
	headers := `HTTP/1.1 200 OK
Content-Type: applicatiom/octet-stream

`
	_, rw, _ := c.Writer.Hijack()
	rw.Writer.Write([]byte(headers))

	for s.LastChunk == "" {
		time.Sleep(time.Second)
	}
	var err error
	for err == nil {
		toRead := s.LastChunk
		f, err := os.Open("."+toRead)
		if err != nil {
			break
		}
		_, err = io.Copy(rw.Writer, f)
		if err != nil {
			break
		}
		s.updateRead()
		for toRead == s.LastChunk {
			time.Sleep(time.Millisecond*100)
		}
	}
}

func (s *Stream) updateRead() {
	s.LastWrite = time.Now()
}


var server Server

func SetHeaders(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin","*")
	c.Next()
}

func checkErr(err error) {
	if err != nil {
		log.Printf("err: %s", err)
		panic(err)
	}
}

func openDB() *sql.DB {
	var db *sql.DB
	log.Printf("opening db")
	_, err := os.Stat(db_file);
	if err != nil {
		db = createDB()
	} else {
		db, err = sql.Open("sqlite3", db_file)
		checkErr(err)
	}
	return db
}

func createDB() *sql.DB {
	log.Printf("creating db")

	db, err := sql.Open("sqlite3", db_file)
        checkErr(err)

	_, err = db.Exec("CREATE TABLE `sources` (`id` INTEGER PRIMARY KEY AUTOINCREMENT, `name` VARCHAR(256) NULL, `url` VARCHAR(256) NOT NULL, `weight` INTEGER NOT NULL DEFAULT 0)")
        checkErr(err)
	log.Printf("created table")

	log.Printf("db created")

	return db
}

func main() {
	router := gin.Default()
	router.Use(SetHeaders)
	router.LoadHTMLGlob("templates/*")

	server = Server{
		Streams: make(map[string]*Stream, 0),
		DB: openDB(),
	}

	router.GET("/", func(c *gin.Context) {
		c.Redirect(307, "/static/list.html")
	})

	router.Static("/static", "./static")
	router.Static("/data", "./data")


	stream := router.Group("/stream/:id", func(c *gin.Context) {
		id := c.Param("id")

		strm := server.getStream(id)
		if strm == nil {
			c.String(500, "Unable to start stream %s", id)
			c.Abort()
		} else {
			c.Set("stream", strm)
		}
	})

	stream.GET("player.html", func(c *gin.Context) {
		id := c.Param("id")
		c.HTML(200, "player.tmpl", gin.H{
			"id": id,
		})
	})

	stream.GET("stream.ts", func(c *gin.Context) {
		strm := c.MustGet("stream").(*Stream)
		strm.serveClient(c)
	})

	stream.GET("hls.m3u8", func(c *gin.Context) {
		strm := c.MustGet("stream").(*Stream)

		strm.updateRead()
		for strm.Playlist == "" {
			time.Sleep(time.Second)
		}
		c.Data(200, "application/x-mpegURL", []byte(strm.Playlist))
	})

	stream.PUT("hls.m3u8", func(c *gin.Context) {
		strm := c.MustGet("stream").(*Stream)
		bodyR := c.Request.Body
		body, _ := ioutil.ReadAll(bodyR)
		sbody := string(body)

		strm.Playlist = sbody
		lines := strings.Split(sbody, "\n")
		strm.LastChunk = lines[len(lines)-2]
	})


	stream.GET("list.m3u", func(c *gin.Context) {
		id := c.Param("id")
		m3u := "#EXTM3U\n"
		m3u = m3u + "#EXTINF:-1,TV\n"
		m3u = m3u + "http://" + c.Request.Host + "/stream/" + id + "/stream.ts\n"
		c.Data(200, "audio/mpegurl", []byte(m3u))
	})

	stream.GET("status", func(c *gin.Context) {
		strm := c.MustGet("stream").(*Stream)
		c.JSON(200, strm)
	})

	router.GET("/status", func(c *gin.Context) {
		c.JSON(200, server)
	})

	router.GET("/sources", func(c *gin.Context) {
		c.JSON(200, server.getSources())
	})

	router.POST("/sources", func(c *gin.Context) {
		var src Source
		err := c.BindJSON(&src)
	        if err != nil {
			c.AbortWithError(400, err)
			return
		}
		if src.Id == 0 {
			err = server.addSource(&src)
		} else {
			err = server.updateSource(&src)
		}
	        if err != nil {
			c.AbortWithError(400, err)
		} else {
			c.String(200, "Updated OK")
		}
	})

	router.DELETE("/sources/:id", func(c *gin.Context) {
		id := c.Param("id")
		iid, err := strconv.Atoi(id)
	        if err != nil {
			c.AbortWithError(400, err)
			return
		}
		src := Source {
			Id: iid,
		}
		err = server.deleteSource(&src)
	        if err != nil {
			c.AbortWithError(400, err)
		} else {
			c.String(200, "Deleted OK")
		}
	})

	router.POST("/sources/csv", func(c *gin.Context) {
		bodyR := c.Request.Body

		err := server.importSourcesCsv(bodyR)
		if err != nil {
			c.String(500, err.Error())
		} else {
			c.String(200, "Imported OK")
		}
	})

	router.GET("/sources/export.m3u", func(c *gin.Context) {
		srcs := server.getSources()

		m3u := "#EXTM3U\n"

		for _,src := range srcs {
			m3u = m3u + "#EXTINF:-1,"+src.Name+"\n"
			m3u = m3u + src.Url+"\n"
		}
		c.Data(200, "audio/mpegurl", []byte(m3u))
	})

	router.GET("/sources/export.csv", func(c *gin.Context) {
		srcs := server.getSources()

		c.Header("Content-Type", "text/csv")
		w := csv.NewWriter(c.Writer)

		for _,src := range srcs {
			if err := w.Write([]string{src.Name,src.Url,strconv.Itoa(src.Weight)}); err != nil {
				log.Printf("error writing record to csv:", err)
			}
		}
	})

	router.Run(":8080")
}
