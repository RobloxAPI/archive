// List elements with conflicting names.
package main

import (
	"encoding/json"
	"fmt"
	"github.com/robloxapi/rbxapi"
	"github.com/robloxapi/rbxapi/rbxapijson"
	"github.com/robloxapi/rbxdhist"
	"os"
	"time"
)

type Nameable interface {
	GetName() string
}

type Builds struct {
	Schema int
	Domain string
	Builds []*Build
}

type Build struct {
	Type    string
	Hash    string
	Date    time.Time
	Version rbxdhist.Version
}

type Conflicts struct {
	Classes   []*ClassConflict
	Members   []*MemberConflict
	Enums     []*EnumConflict
	EnumItems []*EnumItemConflict
}

type ClassConflict struct {
	Build    *Build
	ID       string
	Elements []rbxapi.Class
}

type MemberConflict struct {
	Build    *Build
	ID       string
	Parent   rbxapi.Class
	Elements []rbxapi.Member
}

type EnumConflict struct {
	Build    *Build
	ID       string
	Elements []rbxapi.Enum
}

type EnumItemConflict struct {
	Build    *Build
	ID       string
	Parent   rbxapi.Enum
	Elements []rbxapi.EnumItem
}

func (c *Conflicts) AppendClass(build *Build, elements []rbxapi.Class) {
	lists := make(map[string][]rbxapi.Class)
	for _, item := range elements {
		id := item.GetName()
		lists[id] = append(lists[id], item)
	}
	for _, item := range elements {
		id := item.GetName()
		list := lists[id]
		if len(list) > 1 {
			c.Classes = append(c.Classes, &ClassConflict{build, id, list})
		}
		delete(lists, id)
	}
}

func (c *Conflicts) AppendMember(build *Build, parent rbxapi.Class, elements []rbxapi.Member) {
	lists := make(map[string][]rbxapi.Member)
	for _, item := range elements {
		id := item.GetName()
		lists[id] = append(lists[id], item)
	}
	for _, item := range elements {
		id := item.GetName()
		list := lists[id]
		if len(list) > 1 {
			c.Members = append(c.Members, &MemberConflict{build, id, parent, list})
		}
		delete(lists, id)
	}
}

func (c *Conflicts) AppendEnum(build *Build, elements []rbxapi.Enum) {
	lists := make(map[string][]rbxapi.Enum)
	for _, item := range elements {
		id := item.GetName()
		lists[id] = append(lists[id], item)
	}
	for _, item := range elements {
		id := item.GetName()
		list := lists[id]
		if len(list) > 1 {
			c.Enums = append(c.Enums, &EnumConflict{build, id, list})
		}
		delete(lists, id)
	}
}

func (c *Conflicts) AppendEnumItem(build *Build, parent rbxapi.Enum, elements []rbxapi.EnumItem) {
	lists := make(map[string][]rbxapi.EnumItem)
	for _, item := range elements {
		id := item.GetName()
		lists[id] = append(lists[id], item)
	}
	for _, item := range elements {
		id := item.GetName()
		list := lists[id]
		if len(list) > 1 {
			c.EnumItems = append(c.EnumItems, &EnumItemConflict{build, id, parent, list})
		}
		delete(lists, id)
	}
}

func visitElements(conflicts *Conflicts, build *Build, root rbxapi.Root) {
	classes := root.GetClasses()
	conflicts.AppendClass(build, classes)
	for _, class := range classes {
		conflicts.AppendMember(build, class, class.GetMembers())
	}
	enums := root.GetEnums()
	conflicts.AppendEnum(build, enums)
	for _, enum := range enums {
		conflicts.AppendEnumItem(build, enum, enum.GetItems())
	}
}

func main() {
	f, err := os.Open("../../builds.json")
	if err != nil {
		fmt.Println(err)
		return
	}
	jd := json.NewDecoder(f)
	var builds Builds
	err = jd.Decode(&builds)
	f.Close()
	if err != nil {
		fmt.Println(err)
		return
	}

	var conflicts Conflicts
	for _, build := range builds.Builds {
		if build.Type != "Player" {
			continue
		}
		f, err := os.Open("../../data/api-dump/json/" + build.Hash + ".json")
		if err != nil {
			fmt.Println(err)
			continue
		}
		root, err := rbxapijson.Decode(f)
		f.Close()
		if err != nil {
			fmt.Println(err)
			continue
		}
		visitElements(&conflicts, build, root)
	}
	for _, conflict := range conflicts.Classes {
		fmt.Println(conflict.Build.Version, conflict.Build.Hash, conflict.ID)
		for _, element := range conflict.Elements {
			fmt.Printf("\tClass %s : %s\n", element.GetName(), element.GetSuperclass())
		}
	}
	for _, conflict := range conflicts.Members {
		fmt.Println(conflict.Build.Version, conflict.Build.Hash, conflict.ID)
		for _, element := range conflict.Elements {
			fmt.Printf("\t%s %s.%s\n", element.GetMemberType(), conflict.Parent.GetName(), element.GetName())
		}
	}
	for _, conflict := range conflicts.Enums {
		fmt.Println(conflict.Build.Version, conflict.Build.Hash, conflict.ID)
		for _, element := range conflict.Elements {
			fmt.Printf("\tEnum %s\n", element.GetName())
		}
	}
	for _, conflict := range conflicts.EnumItems {
		fmt.Println(conflict.Build.Version, conflict.Build.Hash, conflict.ID)
		for _, element := range conflict.Elements {
			fmt.Printf("\tEnumItem %s.%s : %d\n", conflict.Parent.GetName(), element.GetName(), element.GetValue())
		}
	}
}
