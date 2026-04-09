package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5"
)

func main() {
	db, err := sql.Open("pgx", "postgres://penkf@localhost/messenger?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	if err = db.PingContext(ctx); err != nil {
		log.Fatal("DB not ready:", err)
	}

	r := gin.Default()
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "pgx+db ready"})
	})

	log.Println("Server :8080")
	r.Run(":8080")
}
