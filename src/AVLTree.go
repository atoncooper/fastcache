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
	y.Left = z   // 将 z 设置为 y 的左子节点
	z.Right = T2 // 将 T2 设置为 z 的右子节点

	z.Height = 1 + max(tree.getHeight(z.Left), tree.getHeight(z.Right)) // 更新 z 的高度
	y.Height = 1 + max(tree.getHeight(y.Left), tree.getHeight(y.Right)) // 更新 y 的高度

	return y // 返回新的根节点 y
}
func (tree *AVLTree) rotateRight(z *Node) *Node {
	y := z.Left   // y 是 z 的左子节点
	T3 := y.Right // T3 是 y 的右子树

	y.Right = z // 将 z 设置为 y 的右子节点
	z.Left = T3 // 将 T3 设置为 z 的左子节点

	z.Height = 1 + max(tree.getHeight(z.Left), tree.getHeight(z.Right)) // 更新 z 的高度
	y.Height = 1 + max(tree.getHeight(y.Left), tree.getHeight(y.Right)) // 更新 y 的高度

	return y // 返回新的根节点 y
}

// 查找节点
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

// 删除节点
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
		// 找到要删除的节点
		if node.Left == nil {
			return node.Right
		} else if node.Right == nil {
			return node.Left
		}

		// 找到右子树的最小值节点
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

// 找到以node为根的子树的最小值节点
func (tree *AVLTree) findMin(node *Node) *Node {
	for node.Left != nil {
		node = node.Left
	}
	return node
}
