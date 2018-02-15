package controller

import (
	"fmt"
	"strconv"

	"bufio"
	"github.com/FactomProject/electiontesting/election"
	"github.com/FactomProject/electiontesting/imessage"
	"github.com/FactomProject/electiontesting/messages"
	"github.com/FactomProject/electiontesting/primitives"
	"github.com/FactomProject/electiontesting/testhelper"
	"os"
	"strings"
)

var _ = fmt.Println

// Controller will be able to route messages to a set of nodes and control various
// communication patterns
type Controller struct {
	AuthSet *testhelper.AuthSetHelper

	Elections  []*election.Election
	Volunteers []*messages.VolunteerMessage

	feds []primitives.Identity
	auds []primitives.Identity

	// Can be all 0s for now
	primitives.ProcessListLocation

	Buffer        *MessageBuffer
	GlobalDisplay *election.Display

	Router          *Router
	OutputsToRouter bool
}

// NewController creates all the elections and initial volunteer messages
func NewController(feds, auds int) *Controller {
	c := new(Controller)
	c.AuthSet = testhelper.NewAuthSetHelper(feds, auds)
	fedlist := c.AuthSet.GetFeds()
	c.Elections = make([]*election.Election, len(fedlist))

	for i, f := range fedlist {
		c.Elections[i] = c.newElection(f)
		if i == 0 {
			c.GlobalDisplay = election.NewDisplay(c.Elections[0], nil)
			c.GlobalDisplay.Identifier = "Global"
		}
		c.Elections[i].AddDisplay(c.GlobalDisplay)
	}

	audlist := c.AuthSet.GetAuds()
	c.Volunteers = make([]*messages.VolunteerMessage, len(audlist))

	for i, a := range audlist {
		c.Volunteers[i] = c.newVolunteer(a)
	}

	c.Buffer = NewMessageBuffer()
	c.feds = fedlist
	c.auds = audlist

	c.Router = NewRouter(c.Elections)
	return c
}

func grabInput(in *bufio.Reader) string {
	input, err := in.ReadString('\n')
	if err != nil {
		fmt.Println("Error: ", err)
		return ""
	}
	return strings.TrimRight(input, "\n")
}

func (c *Controller) Shell() {
	printflipflop := false

	in := bufio.NewReader(os.Stdin)
	for {
		fmt.Print(">")
		input := grabInput(in)

		switch input {
		case "exit":
			fallthrough
		case "quit":
			fallthrough
		case "q":
			return
		case "p":
			printflipflop = !printflipflop
			c.Router.PrintMode(printflipflop)
			fmt.Printf("PrintingSteps: %t\n", printflipflop)
		case "s":
			c.Router.Step()
			fmt.Println("< Steped")
		case "n":
			for i := range c.Elections {
				fmt.Println(c.Router.NodeStack(i))
			}
		case "d":
			num := grabInput(in)
			if num != "a" {
				i, err := strconv.Atoi(num)
				if err != nil {
					fmt.Println(err)
					continue
				}
				fmt.Println(c.ElectionStatus(i))
				continue
			}
			fallthrough
		case "da":
			fmt.Println(c.ElectionStatus(-1))
			for i := range c.Elections {
				fmt.Println(c.ElectionStatus(i))
				fmt.Println(c.Elections[i].VolunteerControlString())
			}
		case "r":
			fmt.Println(c.Router.Status())
		}
	}
}

func (c *Controller) Complete() bool {
	for _, e := range c.Elections {
		if !e.Committed {
			return false
		}
	}
	return true
}

func (c *Controller) SendOutputsToRouter(set bool) {
	c.OutputsToRouter = set
}

func (c *Controller) ElectionStatus(node int) string {
	if node == -1 {
		return c.GlobalDisplay.String()
	}
	return c.Elections[node].Display.String()
}

func (c *Controller) RouteLeaderSetLevelMessage(from []int, level int, to []int) bool {
	for _, f := range from {
		if !c.RouteLeaderLevelMessage(f, level, to) {
			return false
		}
	}
	return true
}

func (c *Controller) RouteLeaderLevelMessage(from int, level int, to []int) bool {
	msg := c.Buffer.RetrieveLeaderLevelMessageByLevel(c.indexToFedID(from), level)
	if msg == nil {
		return false
	}
	c.RouteMessage(msg, to)
	return true
}

func (c *Controller) RouteLeaderSetVoteMessage(from []int, vol int, to []int) bool {
	for _, f := range from {
		if !c.RouteLeaderVoteMessage(f, vol, to) {
			return false
		}
	}
	return true
}

func (c *Controller) RouteLeaderVoteMessage(from int, vol int, to []int) bool {
	msg := c.Buffer.RetrieveLeaderVoteMessage(c.indexToFedID(from), c.indexToAudID(vol))
	if msg == nil {
		return false
	}
	c.RouteMessage(msg, to)
	return true
}

func (c *Controller) RouteVolunteerMessage(vol int, nodes []int) {
	c.RouteMessage(c.Volunteers[vol], nodes)
}

// RouteMessage will execute message on all nodes given and add the messages returned to the buffer
func (c *Controller) RouteMessage(msg imessage.IMessage, nodes []int) {
	for _, n := range nodes {
		c.routeSingleNode(msg, n)
	}
}

func (c *Controller) routeSingleNode(msg imessage.IMessage, node int) {
	resp, _ := c.Elections[node].Execute(msg)

	if c.OutputsToRouter {
		// Outputs get sent to Router so we can hit "run"
		f := messages.GetSigner(msg)
		c.Router.route(c.fedIDtoIndex(f), msg)

	}
	c.Buffer.Add(resp)
}

// indexToAudID will take the human legible "Audit 1" and get the correct identity.
func (c *Controller) indexToAudID(index int) primitives.Identity {
	// TODO: Actually implement some logic if this changes
	return c.auds[index]

}

func (c *Controller) fedIDtoIndex(id primitives.Identity) int {
	for i, f := range c.feds {
		if f == id {
			return i
		}
	}
	return -1
}

// indexToFedID will take the human legible "Leader 1" and get the correct identity
func (c *Controller) indexToFedID(index int) primitives.Identity {
	// TODO: Actually implement some logic if this changes
	return c.feds[index]
}

func (c *Controller) newVolunteer(id primitives.Identity) *messages.VolunteerMessage {
	vol := messages.NewVolunteerMessage(messages.NewEomMessage(id, c.ProcessListLocation), id)
	return &vol
}

func (c *Controller) newElection(id primitives.Identity) *election.Election {
	return election.NewElection(id, c.AuthSet.GetAuthSet(), c.ProcessListLocation)
}