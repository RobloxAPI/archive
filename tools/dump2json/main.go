// Translate original dump files into new JSON format.
package main

import (
	"fmt"
	"github.com/robloxapi/rbxapi"
	"github.com/robloxapi/rbxapi/diff"
	"github.com/robloxapi/rbxapi/rbxapidump"
	"github.com/robloxapi/rbxapi/rbxapijson"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const FixConflicts = true

func VisitTypes(root rbxapi.Root, visit func(rbxapi.Type)) {
	for _, class := range root.GetClasses() {
		visit(rbxapijson.Type{Category: "Class", Name: class.GetName()})
		visit(rbxapijson.Type{Category: "Class", Name: class.GetSuperclass()})
		for _, member := range class.GetMembers() {
			switch member.GetMemberType() {
			case "Property":
				member := member.(rbxapi.Property)
				visit(member.GetValueType())
			case "Function":
				member := member.(rbxapi.Function)
				for _, param := range member.GetParameters() {
					visit(param.GetType())
				}
				visit(member.GetReturnType())
			case "Event":
				member := member.(rbxapi.Event)
				for _, param := range member.GetParameters() {
					visit(param.GetType())
				}
			case "Callback":
				member := member.(rbxapi.Callback)
				for _, param := range member.GetParameters() {
					visit(param.GetType())
				}
				visit(member.GetReturnType())
			}
		}
	}
	for _, enum := range root.GetEnums() {
		visit(rbxapijson.Type{Category: "Enum", Name: enum.GetName()})
	}
}

func VisitClasses(root rbxapi.Root, visit func(rbxapi.Class)) {
	for _, class := range root.GetClasses() {
		visit(class)
	}
}

func VisitMembers(root rbxapi.Root, visit func(rbxapi.Class, rbxapi.Member)) {
	for _, class := range root.GetClasses() {
		for _, member := range class.GetMembers() {
			visit(class, member)
		}
	}
}

func VisitEnums(root rbxapi.Root, visit func(rbxapi.Enum)) {
	for _, enum := range root.GetEnums() {
		visit(enum)
	}
}

func VisitEnumItems(root rbxapi.Root, visit func(rbxapi.Enum, rbxapi.EnumItem)) {
	for _, enum := range root.GetEnums() {
		for _, item := range enum.GetEnumItems() {
			visit(enum, item)
		}
	}
}

func renameTag(tags rbxapidump.Tags, from, to string) rbxapidump.Tags {
	if tags.GetTag(from) {
		tags.UnsetTag(from)
		tags.SetTag(to)
	}
	return tags
}

func getFirst(first *rbxapijson.Root, class, member string) interface{} {
	c := first.GetClass(class)
	if c == nil {
		return nil
	}
	if member == "" {
		return c
	}
	return c.GetMember(member)
}

func PreTransform(root *rbxapidump.Root, first *rbxapijson.Root) {
	foundPages := false
	VisitClasses(root, func(c rbxapi.Class) {
		class := c.(*rbxapidump.Class)
		if FixConflicts {
			// Second instance of Pages class. Was immediately renamed to
			// StandardPages in the next version.
			if class.Name == "Pages" {
				if foundPages {
					class.Name = "StandardPages"
				} else {
					foundPages = true
				}
			}
		}
		class.Tags = renameTag(class.Tags, "notCreatable", "NotCreatable")
		if fclass, _ := getFirst(first, class.Name, "").(*rbxapijson.Class); fclass != nil {
			if fclass.GetTag("NotCreatable") {
				class.SetTag("NotCreatable")
			}
			if fclass.GetTag("Service") {
				class.SetTag("Service")
			}
			if fclass.GetTag("NotReplicated") {
				class.SetTag("NotReplicated")
			}
			if fclass.GetTag("PlayerReplicated") {
				class.SetTag("PlayerReplicated")
			}
		}
		class.Tags = renameTag(class.Tags, "notbrowsable", "NotBrowsable")
		class.Tags = renameTag(class.Tags, "deprecated", "Deprecated")
	})
	VisitMembers(root, func(c rbxapi.Class, member rbxapi.Member) {
		class := c.(*rbxapidump.Class)
		switch member := member.(type) {
		case *rbxapidump.Property:
			member.Tags = renameTag(member.Tags, "hidden", "Hidden")
			member.Tags = renameTag(member.Tags, "readonly", "ReadOnly")
			if fmember, _ := getFirst(first, class.Name, member.Name).(*rbxapijson.Property); fmember != nil {
				if fmember.GetTag("NotReplicated") {
					member.SetTag("NotReplicated")
				}
			}
			member.Tags = renameTag(member.Tags, "notbrowsable", "NotBrowsable")
			member.Tags = renameTag(member.Tags, "deprecated", "Deprecated")
		case *rbxapidump.Function:
			member.Tags = renameTag(member.Tags, "notbrowsable", "NotBrowsable")
			member.Tags = renameTag(member.Tags, "deprecated", "Deprecated")
			if fmember, _ := getFirst(first, class.Name, member.Name).(*rbxapijson.Function); fmember != nil {
				if fmember.GetTag("CustomLuaState") {
					member.SetTag("CustomLuaState")
				}
			}
			if class.Name == "Instance" && member.Name == "WaitForChild" {
				member.SetTag("CanYield")
			}
		case *rbxapidump.YieldFunction:
			member.SetTag("Yields")
			member.Tags = renameTag(member.Tags, "notbrowsable", "NotBrowsable")
			member.Tags = renameTag(member.Tags, "deprecated", "Deprecated")
		case *rbxapidump.Event:
			member.Tags = renameTag(member.Tags, "notbrowsable", "NotBrowsable")
			member.Tags = renameTag(member.Tags, "deprecated", "Deprecated")
		case *rbxapidump.Callback:
			member.Tags = renameTag(member.Tags, "notbrowsable", "NotBrowsable")
			member.Tags = renameTag(member.Tags, "deprecated", "Deprecated")
		}
	})
	foundCameraMode := false
	VisitEnums(root, func(e rbxapi.Enum) {
		enum := e.(*rbxapidump.Enum)
		if FixConflicts {
			// Second instance of CameraMode enum. Was renamed to
			// CustomCameraMode after several versions.
			if enum.Name == "CameraMode" {
				if foundCameraMode {
					enum.Name = "CustomCameraMode"
				} else {
					foundCameraMode = true
				}
			}
		}
		enum.Tags = renameTag(enum.Tags, "notbrowsable", "NotBrowsable")
		enum.Tags = renameTag(enum.Tags, "deprecated", "Deprecated")
	})
	foundRunning := false
	VisitEnumItems(root, func(e rbxapi.Enum, i rbxapi.EnumItem) {
		item := i.(*rbxapidump.EnumItem)
		if FixConflicts {
			// Second instance of Running enum item. Was renamed to
			// RunningNoPhysics after many versions.
			enum := e.(*rbxapidump.Enum)
			if enum.Name == "HumanoidStateType" && item.Name == "Running" {
				if foundRunning {
					item.Name = "RunningNoPhysics"
				} else {
					foundRunning = true
				}
			}
		}
		item.Tags = renameTag(item.Tags, "notbrowsable", "NotBrowsable")
		item.Tags = renameTag(item.Tags, "deprecated", "Deprecated")
	})
}

func transformType(dst *rbxapijson.Type, src *rbxapijson.Type, types *Types) {
	// Try getting category from source.
	if src != nil {
		if dst.Category == "" && src.Name == dst.Name {
			dst.Category = src.Category
		}
	}
	// Try getting category from corpus of known types.
	if dst.Category == "" {
		if ts := types.Get(dst.Name); len(ts) > 0 {
			dst.Category = ts[0].Category
			// If there were more than one type mapped to the name, then we
			// would have to figure out which to use based on the context.
			// Thankfully, no conflicting type names were used in the dump.
		}
	}
	// Rename types that were changed in stable.
	if src != nil {
		// TODO: Renaming these types could be considered a legitimate change.
		// Conversely, the names are unchanged in the dump format of the same
		// version as stable, so they could be considered format-specific.
		switch dst.Name {
		case "EventInstance",
			"Property",
			"CoordinateFrame",
			"Connection",
			"Rect2D":
			dst.Name = src.Name
			// Try getting category from new name, if necessary.
			if dst.Category == "" {
				if ts := types.Get(dst.Name); len(ts) > 0 {
					dst.Category = ts[0].Category
				}
			}
		}
	}
}

func transformParameters(dst *[]rbxapijson.Parameter, src *[]rbxapijson.Parameter, types *Types) {
	if src == nil {
		for i := range *dst {
			transformType(&((*dst)[i].Type), nil, types)
		}
		return
	}
	srcp := *src
	unvisited := make(map[int]struct{}, len(*dst))
	for i := range *dst {
		unvisited[i] = struct{}{}
	}
	for i, p := range *dst {
		for j, fp := range srcp {
			switch {
			case
				fp.Name == p.Name,
				fp.Name != p.Name && i == j:
				transformType(&((*dst)[i].Type), &fp.Type, types)
				delete(unvisited, i)
				break
			}
		}
	}
	for i := range unvisited {
		transformType(&((*dst)[i].Type), nil, types)
	}
}

func PostTransform(jroot, first *rbxapijson.Root, types *Types) {
	VisitClasses(jroot, func(c rbxapi.Class) {
		class := c.(*rbxapijson.Class)
		if class.Name == "Instance" {
			class.Superclass = "<<<ROOT>>>"
		}
		if fclass, _ := getFirst(first, class.Name, "").(*rbxapijson.Class); fclass != nil {
			class.MemoryCategory = fclass.MemoryCategory
		}
	})
	VisitMembers(jroot, func(c rbxapi.Class, m rbxapi.Member) {
		class := c.(*rbxapijson.Class)
		switch member := m.(type) {
		case *rbxapijson.Property:
			if fmember, _ := getFirst(first, class.Name, member.Name).(*rbxapijson.Property); fmember != nil {
				member.Category = fmember.Category
				member.CanLoad = fmember.CanLoad
				member.CanSave = fmember.CanSave
				transformType(&member.ValueType, &fmember.ValueType, types)
			} else {
				transformType(&member.ValueType, nil, types)
			}
			member.WriteSecurity = "None"
			member.ReadSecurity = "None"
			for _, tag := range member.GetTags() {
				const prefix = "ScriptWriteRestricted: ["
				switch {
				case strings.HasPrefix(tag, prefix):
					member.WriteSecurity = tag[len(prefix) : len(tag)-1]
					member.UnsetTag(tag)
				case strings.Contains(tag, "Security"),
					strings.Contains(tag, "security"):
					member.ReadSecurity = tag
					member.WriteSecurity = tag
					member.UnsetTag(tag)
				}
			}
		case *rbxapijson.Function:
			if fmember, _ := getFirst(first, class.Name, member.Name).(*rbxapijson.Function); fmember != nil {
				transformParameters(&member.Parameters, &fmember.Parameters, types)
				transformType(&member.ReturnType, &fmember.ReturnType, types)
			} else {
				transformParameters(&member.Parameters, nil, types)
				transformType(&member.ReturnType, nil, types)
			}
			member.Security = "None"
			for _, tag := range member.GetTags() {
				switch {
				case strings.Contains(tag, "Security"),
					strings.Contains(tag, "security"):
					member.Security = tag
					member.UnsetTag(tag)
				}
			}
		case *rbxapijson.Event:
			if fmember, _ := getFirst(first, class.Name, member.Name).(*rbxapijson.Event); fmember != nil {
				transformParameters(&member.Parameters, &fmember.Parameters, types)
			} else {
				transformParameters(&member.Parameters, nil, types)
			}
			member.Security = "None"
			for _, tag := range member.GetTags() {
				switch {
				case strings.Contains(tag, "Security"),
					strings.Contains(tag, "security"):
					member.Security = tag
					member.UnsetTag(tag)
				}
			}
		case *rbxapijson.Callback:
			if fmember, _ := getFirst(first, class.Name, member.Name).(*rbxapijson.Callback); fmember != nil {
				transformParameters(&member.Parameters, &fmember.Parameters, types)
				transformType(&member.ReturnType, &fmember.ReturnType, types)
			} else {
				transformParameters(&member.Parameters, nil, types)
				transformType(&member.ReturnType, nil, types)
			}
			member.Security = "None"
			for _, tag := range member.GetTags() {
				switch {
				case strings.Contains(tag, "Security"),
					strings.Contains(tag, "security"):
					member.Security = tag
					member.UnsetTag(tag)
				}
			}
		}
	})
}

const Input = `../../data/api-dump/txt`
const Output = `../../data/api-dump/json`

type Types struct {
	Map map[string][]rbxapijson.Type
	sync.Mutex
}

func (t *Types) Get(k string) []rbxapijson.Type {
	t.Lock()
	defer t.Unlock()
	return t.Map[k]
}

func (t *Types) Get2(k string) ([]rbxapijson.Type, bool) {
	t.Lock()
	defer t.Unlock()
	v, ok := t.Map[k]
	return v, ok
}

func (t *Types) Set(k string, v []rbxapijson.Type) {
	t.Lock()
	defer t.Unlock()
	t.Map[k] = v
}

func main() {
	// Backport unavailable fields with the first stable JSON dump.
	f, err := os.Open(`../stable.json`)
	if err != nil {
		fmt.Println("failed to open stable dump:", err)
	}
	stable, err := rbxapijson.Decode(f)
	f.Close()
	if err != nil {
		fmt.Println("failed to decode stable dump:", err)
		return
	}

	if err := os.MkdirAll(Output, 0666); err != nil {
		fmt.Println("failed to make output directory:", err)
		return
	}

	dirs, err := ioutil.ReadDir(Input)
	if err != nil {
		fmt.Println("failed to read input directory:", err)
		return
	}

	// Manually add types that had been removed at some point.
	types := &Types{
		Map: map[string][]rbxapijson.Type{
			// Replaced by "Class:<class>".
			"Object": {{Category: "DataType", Name: "Object"}},
			// Renamed to "CFrame".
			"CoordinateFrame": {{Category: "DataType", Name: "CoordinateFrame"}},
			// All references removed.
			"SystemAddress":        {{Category: "DataType", Name: "SystemAddress"}},
			"BuildPermission":      {{Category: "Enum", Name: "BuildPermission"}},
			"PhysicsReceiveMethod": {{Category: "Enum", Name: "PhysicsReceiveMethod"}},
			"PhysicsSendMethod":    {{Category: "Enum", Name: "PhysicsSendMethod"}},
			"PrismSides":           {{Category: "Enum", Name: "PrismSides"}},
			"PyramidSides":         {{Category: "Enum", Name: "PyramidSides"}},
		},
	}
	typeVisitor := func(typ rbxapi.Type) {
		types.Lock()
		defer types.Unlock()
		cat := typ.GetCategory()
		if cat == "" {
			return
		}
		name := typ.GetName()
		ts := types.Map[name]
		for _, t := range ts {
			if t.GetCategory() == cat {
				return
			}
		}
		types.Map[name] = append(types.Map[name], rbxapijson.Type{Category: typ.GetCategory(), Name: typ.GetName()})
	}
	VisitTypes(stable, typeVisitor)

	var wg sync.WaitGroup
	for _, file := range dirs {
		wg.Add(1)
		go func(file os.FileInfo) {
			defer wg.Done()
			name := file.Name()
			base := name[:len(name)-len(filepath.Ext(name))]
			fmt.Println("starting ", base)
			f, err := os.Open(filepath.Join(Input, name))
			if err != nil {
				fmt.Println("failed to open input ", name, ":", err)
				return
			}
			root, err := rbxapidump.Decode(f)
			f.Close()
			if err != nil {
				fmt.Println("failed to decode ", name, ":", err)
				return
			}

			PreTransform(root, stable)
			// In theory, every type in every build should be visited first,
			// or else types that are added in a particular version cannot be
			// backported to previous versions. In practice, such case are
			// taken care of almost entirely by having visited stable first.
			// Only a handful of types were removed, and do not appear in
			// stable, but can be added back manually.
			VisitTypes(root, typeVisitor)
			jroot := &rbxapijson.Root{}
			jroot.Patch((&diff.Diff{Prev: &rbxapidump.Root{}, Next: root}).Diff())
			PostTransform(jroot, stable, types)
			if FixConflicts {
				// Many versions saw a DataModel.Loaded function, which
				// conflicted with the Loaded event. Apparently it went
				// unused, and so was ultimately removed. The same is done
				// here. Although it might be worth renaming it instead, there
				// isn't any specific name that can be used.
				if class, _ := jroot.GetClass("DataModel").(*rbxapijson.Class); class != nil {
					for i := 0; i < len(class.Members); {
						if class.Members[i].GetMemberType() == "Function" && class.Members[i].GetName() == "Loaded" {
							copy(class.Members[i:], class.Members[i+1:])
							class.Members[len(class.Members)-1] = nil
							class.Members = class.Members[:len(class.Members)-1]
						} else {
							i++
						}
					}
				}
				// Many versions saw a number of redundant
				// KeyCode.KeypadEquals enum items. All the extras were
				// eventually removed, so they're not very interesting for
				// keeping around.
				if enum, _ := jroot.GetEnum("KeyCode").(*rbxapijson.Enum); enum != nil {
					foundKeypadEquals := false
					for i := 0; i < len(enum.Items); i++ {
						if enum.Items[i].Name == "KeypadEquals" {
							if foundKeypadEquals {
								copy(enum.Items[i:], enum.Items[i+1:])
								enum.Items[len(enum.Items)-1] = nil
								enum.Items = enum.Items[:len(enum.Items)-1]
								i--
							} else {
								foundKeypadEquals = true
							}
						}
					}
				}
			}

			if f, err = os.Create(filepath.Join(Output, base+".json")); err != nil {
				fmt.Println("failed to open output ", name, ":", err)
			}
			if err := rbxapijson.Encode(f, jroot); err != nil {
				fmt.Println("failed to encode ", name, ":", err)
			}
			f.Close()
			fmt.Println("finished ", base)
		}(file)
	}
	wg.Wait()
	fmt.Println("done")
}
