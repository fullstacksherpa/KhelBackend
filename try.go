package main

import (
	"fmt"
	"strings"
)

// extractPublicIDFromURL extracts the public ID from a Cloudinary URL
func extractPublicIDFromURL(url string) string {
	// Split the URL by "/"
	parts := strings.Split(url, "/")

	// Find the index of "upload" in the URL
	uploadIndex := -1
	for i, part := range parts {
		if part == "upload" {
			uploadIndex = i
			break
		}
	}

	// If "upload" is not found, return an empty string
	if uploadIndex == -1 || uploadIndex >= len(parts)-1 {
		return ""
	}

	// The public ID starts after the version number (e.g., "v1740815725")
	// So we skip the version number and take everything after it
	publicIDWithExtension := strings.Join(parts[uploadIndex+2:], "/")

	// Remove the file extension (e.g., ".png", ".jpg")
	publicID := strings.TrimSuffix(publicIDWithExtension, ".png")
	publicID = strings.TrimSuffix(publicID, ".jpg") // Add more extensions if needed

	return publicID
}

func main() {
	// Example Cloudinary URLs
	urls := []string{
		"https://res.cloudinary.com/devsherpa/image/upload/v1740815725/venues/ay2av1mwuakrobwzv0vl.png",
		"https://res.cloudinary.com/devsherpa/image/upload/v1740815726/venues/oifa92bjwmw4kjvrhhdn.png",
	}

	// Extract and print the public IDs
	for _, url := range urls {
		publicID := extractPublicIDFromURL(url)
		fmt.Printf("URL: %s\nPublic ID: %s\n\n", url, publicID)
	}
}

// sherpa.personall Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJLaGVsIiwiZXhwIjoxNzQyNTEzODYxLCJpYXQiOjE3NDIyNTQ2NjEsImlzcyI6IktoZWwiLCJuYmYiOjE3NDIyNTQ2NjEsInN1YiI6MTF9.atkPJnh7qewGobFv2dKVHDJT960aVqlH0gl9Ng4_cnE

// npx autocannon -r 8000 -d 2 -c 10 --renderStatusCodes http://localhost:8080/v1/health

//npx autocannon http://localhost:8080/v1/get-games -r 2 --connections 5 --duration 5 -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJLaGVsIiwiZXhwIjoxNzQyNTEzODYxLCJpYXQiOjE3NDIyNTQ2NjEsImlzcyI6IktoZWwiLCJuYmYiOjE3NDIyNTQ2NjEsInN1YiI6MTF9.atkPJnh7qewGobFv2dKVHDJT960aVqlH0gl9Ng4_cnE"
