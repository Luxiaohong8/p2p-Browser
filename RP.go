package main

import (
	log "github.com/sirupsen/logrus"
	"errors"
	"sort"
)

func (this *Hub) GenParents(node *Client) (parents []*Client)  {
	parents = make([]*Client, 0)
	layerResult, err := this.filterByLayer(node)

	if err != nil {
		log.Println(err)
		return parents
	}
	if len(layerResult) < this.P2pConfig.Live.MaxRPNodes {
		return layerResult
	}
	//继续通过剩余带宽过滤
	BWResult, err := this.filterByResidualBW(layerResult, this.P2pConfig.Live.RPBWThreshold)
	if err != nil {
		return layerResult[:this.P2pConfig.Live.MaxRPNodes]
	}
	if len(BWResult) < this.P2pConfig.Live.MaxRPNodes {
		return BWResult
	}
	//继续通过ISP过滤
	ISPResult, err := this.filterByISP(BWResult, node.IpInfo.ISP)
	if err != nil {
		return BWResult[:this.P2pConfig.Live.MaxRPNodes]
	}
	if len(ISPResult) < this.P2pConfig.Live.MaxRPNodes {
		return ISPResult
	}
	//继续通过省份过滤
	pronvResult, err := this.filterByProvince(ISPResult, node.IpInfo.Province)
	if err != nil {
		return ISPResult[:this.P2pConfig.Live.MaxRPNodes]
	}
	if len(pronvResult) < this.P2pConfig.Live.MaxRPNodes {
		return pronvResult
	}
	return pronvResult[:this.P2pConfig.Live.MaxRPNodes]
}

// 通过layer过滤节点，先筛选layer等于此节点的，如果数量够多再筛选layer小于此节点的
func (this *Hub) filterByLayer(c *Client) (nodes []*Client, err error) {
	//if c.treeNode.layer == 0 {
	//	c.treeNode.layer = uint(this.P2pConfig.Live.MaxLayers-1)
	//}
	if c.treeNode.layer == 0 {                                          //孤立节点可以接受所有节点作为父节点
		this.clients.Range(func(key, value interface{}) bool {
			if key != c.PeerId {
				nodes = append(nodes, value.(*Client))
			}
			return true
		})
	} else {
		this.clients.Range(func(key, value interface{}) bool {
			if value.(*Client).treeNode.layer <= c.treeNode.layer && key != c.PeerId {
				nodes = append(nodes, value.(*Client))
			}
			return true
		})
	}
	if len(nodes) == 0 {
		return nil, errors.New("no nodes available")
	}
	//var secondary []*Client
	//if len(nodes) > this.P2pConfig.Live.MaxRPNodes {
	//	for _, v := range nodes {
	//		if v.treeNode.layer < c.treeNode.layer {
	//			secondary = append(secondary, v)
	//		}
	//	}
	//}
	//if len(secondary) >= this.P2pConfig.Live.MinRPNodes {
	//	return secondary, nil
	//}
	return nodes, nil
}

// 通过剩余带宽过滤节点，过滤掉阈值小于threshold的节点
func (this *Hub) filterByResidualBW(source []*Client, threshold int64) (nodes []*Client, err error) {
	for _, v := range source {
		//log.Printf("v.IpInfo.ISP %s", v.IpInfo.ISP)
		//log.Printf("v.id %s residualBW: %v", v.PeerId, v.ResidualBW)
		if v.ResidualBW >= threshold {
			nodes = append(nodes, v)
		}
		//根据BW排序
		log.Printf("before sort npdes length %v", len(nodes))
		SortNodes(nodes, func (p, q *Client) bool{
			return p.ResidualBW > q.ResidualBW
		})
		log.Printf("after sort npdes length %v", len(nodes))
		for _, v := range nodes {
			log.Printf("[filterByResidualBW] peerId %v residualBW %v", v.PeerId, v.ResidualBW)
		}

	}
	if len(nodes) >= this.P2pConfig.Live.MinRPNodes {
		return nodes, nil
	}
	return nil, errors.New("no enough nodes")
}

// 通过ISP过滤节点
func (this *Hub) filterByISP(source []*Client, ISP string) (nodes []*Client, err error) {
	for _, v := range source {
		log.Printf("v.IpInfo.ISP %s", v.IpInfo.ISP)
		log.Printf("v.id %s", v.PeerId)
		if v.IpInfo.ISP == ISP {

			nodes = append(nodes, v)
		}
	}
	//log.Printf("len %d", len(nodes))
	if len(nodes) >= this.P2pConfig.Live.MinRPNodes {
		return nodes, nil
	}
	return nil, errors.New("no enough nodes")
}

// 通过地域(省份)过滤节点
func (this *Hub) filterByProvince(source []*Client, pronv string) (nodes []*Client, err error) {
	for _, v := range source {
		if v.IpInfo.Province == pronv {
			nodes = append(nodes, v)
		}
	}
	if len(nodes) >= this.P2pConfig.Live.MinRPNodes {
		return nodes, nil
	}
	return nil, errors.New("no enough nodes")
}

func (c *Client) getResidualBW() int64 {
	ul_srs := c.Stat.Ul_srs
	var sr_sum int64
	for _, ul_sr := range ul_srs {
		sr_sum += ul_sr
	}
	log.Printf("[getResidualBW] id %s UploadBW %v sr_sum %v", c.PeerId, c.UploadBW, sr_sum)
	return c.UploadBW - sr_sum
}

type NodesWrapper struct {
	nodes []*Client
	by func(p, q * Client) bool
}

type SortBy func(p, q *Client) bool

func (nw NodesWrapper) Len() int {         // 重写 Len() 方法
	return len(nw.nodes)
}

func (nw NodesWrapper) Swap(i, j int){     // 重写 Swap() 方法
	nw.nodes[i], nw.nodes[j] = nw.nodes[j], nw.nodes[i]
}

func (nw NodesWrapper) Less(i, j int) bool {    // 重写 Less() 方法
	return nw.by(nw.nodes[i], nw.nodes[j])
}

func SortNodes(nodes [] *Client, by SortBy){
	sort.Sort(NodesWrapper{nodes, by})
}

