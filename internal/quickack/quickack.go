// Package quickack makes a listener whose connections acknowledge what
// they are sent at once (0145).
//
// An AP's HTTP client, uclient, writes one request as several small
// pieces: the request line, each header, the body. TCP on the AP sends the
// first and holds the rest until the first is acknowledged (Nagle's
// algorithm), and TCP on the manager, which has nothing to send yet, waits
// 40 ms before it acknowledges (delayed acknowledgement). Every request
// from an AP then takes 40 ms longer than the network needs, which is
// nothing to a poll once a minute and is most of the time of a word an AP
// sends up its line as something happens. The AP's side cannot be set from
// ucode. Linux lets the manager's side be: TCP_QUICKACK, which holds only
// until the connection next looks like a conversation, so it is set again
// after every read and every write.
//
// Elsewhere than Linux the listener is given back as it is.
package quickack
