// Fixture: EVO-UI-004 must stay silent. Plain text with no escape and no
// status tag, and a status word that is data rather than a tag.
package ui004

import "fmt"

func report(name string) {
	fmt.Println("checked", name)
	fmt.Println("tags: [ok-list]", name)
}
