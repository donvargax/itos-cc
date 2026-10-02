package main

import (
	"example.com/api/internal/store"
	"fmt"
	"net/http"
)

func main() {
	http.HandleFunc("GET /api/invoices/{id}", invoice)
	fmt.Println(store.Get())
}

func invoice(w http.ResponseWriter, r *http.Request) {}
