package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"ek-server/internal/network"

	"github.com/joho/godotenv"
)

func main() {
	// Cargar variables de entorno (.env)
	if err := godotenv.Load(); err != nil {
		log.Println("Aviso: no se encontró archivo .env, se usará configuración por defecto")
	}

	// Start the cleanup routine for disconnected players
	network.StartCleanupRoutine()

	// Websocket route
	http.HandleFunc("/ws", network.HandleConnections)

	// Start the server
	log.Println("Starting server on port 8080")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("Websocket server started on port", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal("ListenAndServe failed: ", err)
	}
}
