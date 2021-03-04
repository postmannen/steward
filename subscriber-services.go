package steward

// // subscriberServices will hold all the helper services needed for
// // the different subcribers. Example of a help service can be a log
// // subscriber needs a way to write logs locally or send them to some
// // other central logging system.
// type subscriberServices struct {
// 	// sayHelloNodes are the register where the register where nodes
// 	// who have sent an sayHello are stored. Since the sayHello
// 	// subscriber is a handler that will be just be called when a
// 	// hello message is received we need to store the metrics somewhere
// 	// else, that is why we store it here....at least for now.
// 	sayHelloNodes map[node]struct{}
// }
//
// //newSubscriberServices will prepare and return a *subscriberServices
// func newSubscriberServices() *subscriberServices {
// 	s := subscriberServices{
// 		sayHelloNodes: make(map[node]struct{}),
// 	}
//
// 	return &s
// }
//
// // ---
//
