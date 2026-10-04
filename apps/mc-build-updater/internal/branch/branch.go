package branch

import "fmt"

func Print(name string) {
	switch name {
	case "dead-inside-land":
		fmt.Println("DEAD INSIDE LAND")
	case "neko-land":
		fmt.Println("NEKO LAND!")
	default:
		fmt.Printf("Branch: %s\n", name)
	}
}
