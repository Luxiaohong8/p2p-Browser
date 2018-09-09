package main

import "log"

const UINT_MAX = ^uint(0)

type TreeNode struct {
	id       string
	layer    uint
	parents  []*TreeNode
	children []*TreeNode
}

type FastMesh struct {
}

func (this *FastMesh) AddEdge(parent, child *TreeNode) {
	parent.children = append(parent.children, child)
	child.parents = append(child.parents, parent)
	//递归计算新的layer
	this.UpdateLayer(child)
}

func (this *FastMesh) DeleteEdge(parent, child *TreeNode) {
	for i, v := range parent.children {
		if v == child {
			parent.children = append(parent.children[:i], parent.children[i+1:]...)

		}
	}
	for i, v := range child.parents {
		if v == parent {
			child.parents = append(child.parents[:i], child.parents[i+1:]...)
			//递归计算新的layer
			//this.UpdateLayer(child)
		}
	}
}

func (this *FastMesh) UpdateLayer(node *TreeNode) {
	oldLayer := node.layer
	if len(node.parents) == 0 {
		node.layer = 0
	} else {
		newLayer := UINT_MAX
		for _, v := range node.parents {
			if v.layer+1 < newLayer {
				newLayer = v.layer + 1
			}
		}
		node.layer = newLayer
	}
	log.Printf("node %s layer: %d", node.id, node.layer)
	if len(node.children) > 0 {
		for _, c := range node.children {
			if c.layer > oldLayer { //防止死循环
				this.UpdateLayer(c)
			}
		}
	}
}
