package jellybean

import (
	"log"
	"reflect"
)

func MustParse(dest any) {
	v := reflect.ValueOf(dest)
	t := reflect.TypeOf(dest)

	if t.Kind() != reflect.Ptr {
		log.Fatalf("%s is not a pointer (did you forget an ampersand?)", t)
	}
	log.Printf("%v: %T, %v", v, v, t)
}
