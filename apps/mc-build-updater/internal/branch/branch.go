package branch

import "fmt"

func Print(name string) {
	switch name {
	case "dead-inside-land":
		fmt.Println("Ветка: DEAD INSIDE LAND")
	case "neko-land":
		fmt.Println("Ветка: NEKO LAND!")
	default:
		fmt.Printf("Ветка: %s\n", name)
	}
}
