package src

type AVLTree struct {
	Root *Node
}
type Node struct {
	Key    int
	Value  any
	Left   *Node
	Right  *Node
	Height int
}

func (tree *AVLTree) AddNode(key int, value any) {
	tree.Root = tree.addNode(tree.Root, key, value)
}

func (tree *AVLTree) addNode(node *Node, key int, value any) *Node {
	if node == nil {
		return &Node{Key: key, Value: value, Height: 1}
	}
	if key < node.Key {
		node.Left = tree.addNode(node.Left, key, value)
	}
	if key > node.Key {
		node.Right = tree.addNode(node.Right, key, value)
	}
	node.Height = 1 + max(tree.getHeight(node.Left), tree.getHeight(node.Right))
	node.Height = 1 + max(tree.getHeight(node.Left), tree.getHeight(node.Right))
	return tree.balance(node)
}

func (tree *AVLTree) getHeight(node *Node) int {
	if node == nil {
		return 0
	}
	return node.Height
}

func (tree *AVLTree) balance(node *Node) *Node {
	balance := tree.getBalance(node)
	if balance > 1 {
		if tree.getBalance(node.Left) < 0 {
			node.Left = tree.rotateLeft(node.Left)
		}
		return tree.rotateRight(node)
	}
	if balance < -1 {
		if tree.getBalance(node.Right) > 0 {
			node.Right = tree.rotateRight(node.Right)
		}
		return tree.rotateLeft(node)
	}
	return node
}

func (tree *AVLTree) getBalance(node *Node) int {
	if node == nil {
		return 0
	}
	return tree.getHeight(node.Left) - tree.getHeight(node.Right)
}
func (tree *AVLTree) rotateLeft(z *Node) *Node {
	y := z.Right
	T2 := z.Left
	y.Left = z   // Set z as y's left child
	z.Right = T2 // Set T2 as z's right child

	z.Height = 1 + max(tree.getHeight(z.Left), tree.getHeight(z.Right)) // Update z's height
	y.Height = 1 + max(tree.getHeight(y.Left), tree.getHeight(y.Right)) // Update y's height

	return y // Return new root node y
}
func (tree *AVLTree) rotateRight(z *Node) *Node {
	y := z.Left   // y is z's left child
	T3 := y.Right // T3 is y's right subtree

	y.Right = z // Set z as y's right child
	z.Left = T3 // Set T3 as z's left child

	z.Height = 1 + max(tree.getHeight(z.Left), tree.getHeight(z.Right)) // Update z's height
	y.Height = 1 + max(tree.getHeight(y.Left), tree.getHeight(y.Right)) // Update y's height

	return y // Return new root node y
}

// Find finds a node.
func (tree *AVLTree) Find(key int) (any, bool) {
	return tree.findNode(tree.Root, key)
}

func (tree *AVLTree) findNode(node *Node, key int) (any, bool) {
	if node == nil {
		return nil, false
	}
	if node.Key == key {
		return node.Value, true
	}
	if key < node.Key {
		return tree.findNode(node.Left, key)
	}
	return tree.findNode(node.Right, key)
}

// Delete deletes a node.
func (tree *AVLTree) Delete(key int) {
	tree.Root = tree.deleteNode(tree.Root, key)
}

func (tree *AVLTree) deleteNode(node *Node, key int) *Node {
	if node == nil {
		return nil
	}
	if key < node.Key {
		node.Left = tree.deleteNode(node.Left, key)
	} else if key > node.Key {
		node.Right = tree.deleteNode(node.Right, key)
	} else {
		// Find the node to delete
		if node.Left == nil {
			return node.Right
		} else if node.Right == nil {
			return node.Left
		}

		// Find the minimum node in the right subtree
		minNode := tree.findMin(node.Right)
		node.Key = minNode.Key
		node.Value = minNode.Value
		node.Right = tree.deleteNode(node.Right, minNode.Key)
	}
	if node == nil {
		return nil
	}
	node.Height = 1 + max(tree.getHeight(node.Left), tree.getHeight(node.Right))
	return tree.balance(node)
}

// findMin finds the minimum node in the subtree rooted at node.
func (tree *AVLTree) findMin(node *Node) *Node {
	for node.Left != nil {
		node = node.Left
	}
	return node
}
