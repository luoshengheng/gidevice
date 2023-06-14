package nskeyedarchiver

import "howett.net/plist"

type NSSet struct {
	internal []interface{}
}

func NewNSSet(value []interface{}) *NSSet {
	return &NSSet{
		internal: value,
	}
}

func (ns *NSSet) archive(objects []interface{}) []interface{} {
	objs := make([]interface{}, 0, len(ns.internal))

	info := map[string]interface{}{}
	objects = append(objects, info)

	for _, v := range ns.internal {
		var uid plist.UID
		objects, uid = archive(objects, v)
		objs = append(objs, uid)
	}

	info["NS.objects"] = objs
	info["$class"] = plist.UID(len(objects))

	cls := map[string]interface{}{
		"$classname": "NSSet",
		"$classes":   []interface{}{"NSSet", "NSObject"},
	}
	objects = append(objects, cls)

	return objects
}
