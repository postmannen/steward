package steward

// // REMOVED:
// type publisherServices struct {
// 	sayHelloPublisher sayHelloPublisher
// }
//
// func newPublisherServices(sayHelloInterval int) *publisherServices {
// 	ps := publisherServices{
// 		sayHelloPublisher: sayHelloPublisher{
// 			interval: sayHelloInterval,
// 		},
// 	}
// 	return &ps
// }
//
// // ---
//
// type sayHelloPublisher struct {
// 	interval int
// }
//
// func (s *sayHelloPublisher) start(newMessagesCh chan<- []subjectAndMessage, fromNode node) {
// 	go func() {
// 		for {
// 			sam := s.createMsg(fromNode)
// 			newMessagesCh <- []subjectAndMessage{sam}
// 			time.Sleep(time.Second * time.Duration(s.interval))
// 		}
// 	}()
// }
//
// // Will prepare a subject and message with the content
// // of the error
// func (s *sayHelloPublisher) createMsg(FromNode node) subjectAndMessage {
// 	// TESTING: Creating an error message to send to errorCentral
// 	m := fmt.Sprintf("Hello from %v\n", FromNode)
//
// 	sam := subjectAndMessage{
// 		Subject: Subject{
// 			ToNode:         "central",
// 			CommandOrEvent: EventNACK,
// 			Method:         SayHello,
// 		},
// 		Message: Message{
// 			ToNode:   "central",
// 			FromNode: FromNode,
// 			Data:     []string{m},
// 			Method:   SayHello,
// 		},
// 	}
//
// 	return sam
// }
//
