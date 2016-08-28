package main

import (
	"encoding/hex"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

type Section struct {
	StartIndex int
	EndIndex   int
	Name       string    `json:"text,omitempty"`
	Children   []Section `json:"children,omitempty"`
	Id         int       `json:"id"`
}

type Page struct {
	ClassFileBytes []string
	JavaSource     string
	Err            error
	Class          []Section
}

func main() {
	classFile, _ := ioutil.ReadFile("static/HelloWorld.class")

	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("$PORT must be set")
	}

	r := gin.Default()
	r.LoadHTMLGlob("templates/*.tmpl*")
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl.html", nil)
	})
	r.GET("/class", func(c *gin.Context) {
		c.JSON(http.StatusOK, classJSON(classFile))
	})
	r.Run(":" + port)
}

func classJSON(classFile []byte) gin.H {
	result := gin.H{}
	hexString := hex.EncodeToString(classFile)
	var classString []string
	len := len(hexString)
	for i := 0; i < len; i += 2 {
		classString = append(classString, hexString[i:i+2])
	}
	result["raw"] = classString
	result["parsed"] = parseClass(classFile)
	return result
}
