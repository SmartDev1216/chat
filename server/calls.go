/******************************************************************************
 *
 *  Description :
 *    Video call handling (establishment, metadata exhange and termination).
 *
 *****************************************************************************/
package main

import (
	"strconv"
	"time"

	"github.com/tinode/chat/server/logs"
	"github.com/tinode/chat/server/store/types"
)

// Video call constants.
const (
	// Events for call between users A and B.
	//
	// Call started (A is dialing B).
	constCallEventInvite = "invite"
	// B has received the call but hasn't picked it up yet.
	constCallEventRinging = "ringing"
	// B has accepted the call.
	constCallEventAccept = "accept"
	// WebRTC SDP & ICE data exchange events.
	constCallEventOffer        = "offer"
	constCallEventAnswer       = "answer"
	constCallEventIceCandidate = "ice-candidate"
	// Call finished by either side or server.
	constCallEventHangUp = "hang-up"

	// Messages representing call states.
	// Call is established.
	constCallMsgAccepted = "accepted"
	// Call in progress has successfully finished.
	constCallMsgFinished = "finished"
	// Call is dropped.
	constCallMsgDisconnected = "disconnected"

	// How long the server will wait for call establishment
	// after call initiation before it drops the call.
	constCallEstablishmentTimeout = 15 * time.Second
	// Call message mime type.
	constTinodeVideoCallMimeType = "application/x-tinode-webrtc"
)

// videoCall describes video call that's being established or in progress.
type videoCall struct {
	// Call participants.
	parties map[*Session]callPartyData
	// Call message seq ID.
	seq int
}

func (call *videoCall) messageHead() map[string]interface{} {
	head := make(map[string]interface{})
	head["mime"] = constTinodeVideoCallMimeType
	head["replace"] = ":" + strconv.Itoa(call.seq)
	return head
}

// Generates server info message template for the video call event.
func (call *videoCall) infoMessage(event string) *ServerComMessage {
	return &ServerComMessage{
		Info: &MsgServerInfo{
			What:  "call",
			Event: event,
			SeqId: call.seq,
		},
	}
}

// Returns Uid and session of the present video call originator
// if a call is being established or in progress.
func (t *Topic) getCallOriginator() (types.Uid, *Session) {
	if t.currentCall == nil {
		return types.ZeroUid, nil
	}
	for sess, p := range t.currentCall.parties {
		if p.isOriginator {
			return p.uid, sess
		}
	}
	return types.ZeroUid, nil
}

// Handles video call invite (initiation)
// (in response to msg = {pub head=[mime: application/x-tiniode-webrtc]}).
func (t *Topic) handleCallInvite(msg *ClientComMessage, asUid types.Uid) {
	if t.currentCall != nil {
		// There's already another call in progress.
		msg.sess.queueOut(ErrCallBusyReply(msg, types.TimeNow()))
		return
	}
	if t.cat != types.TopicCatP2P {
		msg.sess.queueOut(ErrPermissionDeniedReply(msg, types.TimeNow()))
		return
	}

	tgt := t.p2pOtherUser(asUid)
	t.infoCallSubsOffline(msg.AsUser, tgt, constCallEventInvite, t.lastID, nil, msg.sess.sid, false)
	// Call being establshed.
	t.currentCall = &videoCall{
		parties: make(map[*Session]callPartyData),
		seq:     t.lastID,
	}
	t.currentCall.parties[msg.sess] = callPartyData{
		uid:          asUid,
		isOriginator: true,
	}
	// Wait for constCallEstablishmentTimeout for the other side to accept the call.
	t.callEstablishmentTimer.Reset(constCallEstablishmentTimeout)
}

// Handles events on existing video call (acceptance, termination, metadata exchange).
// (in response to msg = {note what=call}).
func (t *Topic) handleCallEvent(msg *ClientComMessage) {
	if t.currentCall == nil {
		// Must initiate call first.
		//msg.sess.queueOut(ErrOperationNotAllowedReply(msg, types.TimeNow()))
		return
	}
	if t.isInactive() {
		// Topic is paused or being deleted.
		//msg.sess.queueOut(ErrCallBusyReply(msg, types.TimeNow()))
		return
	}

	call := msg.Note
	if t.currentCall.seq != call.SeqId {
		// Call not found.
		//msg.sess.queueOut(ErrNotFoundReply(msg, types.TimeNow()))
		return
	}

	asUid := types.ParseUserId(msg.AsUser)

	_, userFound := t.perUser[asUid]
	if !userFound {
		// User not found in topic.
		//msg.sess.queueOut(ErrNotFoundReply(msg, types.TimeNow()))
		return
	}

	switch call.Event {
	case constCallEventRinging, constCallEventAccept:
		// Invariants:
		// 1. Call has been initiated but not been established yet.
		if len(t.currentCall.parties) != 1 {
			//msg.sess.queueOut(ErrOperationNotAllowedReply(msg, types.TimeNow()))
			return
		}
		originatorUid, originator := t.getCallOriginator()
		if originator == nil {
			logs.Warn.Printf("topic[%s]: video call (seq %d) has no originator. Terminating.", t.name, t.currentCall.seq)
			//msg.sess.queueOut(ErrOperationNotAllowedReply(msg, types.TimeNow()))
			t.terminateCallInProgress()
			return
		}
		// 2. These events may only arrive from the callee.
		if originator == msg.sess || originatorUid == asUid {
			//msg.sess.queueOut(ErrOperationNotAllowedReply(msg, types.TimeNow()))
			return
		}
		// Prepare a {info} message to forward to the call originator.
		forwardMsg := t.currentCall.infoMessage(call.Event)
		forwardMsg.Info.From = msg.AsUser
		forwardMsg.Info.Topic = t.original(originatorUid)
		if call.Event == constCallEventAccept {
			// The call has been accepted.
			// Send a replacement {data} message to the topic.
			replaceWith := constCallMsgAccepted
			head := t.currentCall.messageHead()
			msgCopy := *msg
			msgCopy.AsUser = originatorUid.UserId()
			if err := t.saveAndBroadcastMessage(&msgCopy, originatorUid, false, nil,
				head, replaceWith); err != nil {
				return
			}
			// Add callee data to t.currentCall.
			t.currentCall.parties[msg.sess] = callPartyData{
				uid:          asUid,
				isOriginator: false,
			}

			// Notify other clients that the call has been accepted.
			t.infoCallSubsOffline(msg.AsUser, asUid, call.Event, t.lastID, call.Payload, msg.sess.sid, false)
			t.callEstablishmentTimer.Stop()
		}
		originator.queueOut(forwardMsg)
	case constCallEventOffer, constCallEventAnswer, constCallEventIceCandidate:
		// Call metadata exchange. Either side of the call may send these events.
		// Simply forward them to the other session.
		var otherUid types.Uid
		var otherEnd *Session
		for sess, p := range t.currentCall.parties {
			if sess != msg.sess {
				otherUid = p.uid
				otherEnd = sess
				break
			}
		}
		if otherEnd == nil {
			//msg.sess.queueOut(ErrUserNotFoundReply(msg, types.TimeNow()))
			return
		}
		// All is good.
		//msg.sess.queueOut(NoErrReply(msg, types.TimeNow()))

		// Send {info} message to the otherEnd.
		forwardMsg := t.currentCall.infoMessage(call.Event)
		forwardMsg.Info.From = msg.AsUser
		forwardMsg.Info.Topic = t.original(otherUid)
		forwardMsg.Info.Payload = call.Payload
		otherEnd.queueOut(forwardMsg)
	case constCallEventHangUp:
		t.maybeEndCallInProgress(msg.AsUser, msg)
	default:
		logs.Warn.Printf("topic[%s]: video call (seq %d) received unexpected call event: %s", t.name, t.currentCall.seq, call.Event)
	}
}

// Ends current call in response to a client hangup request (msg).
func (t *Topic) maybeEndCallInProgress(from string, msg *ClientComMessage) {
	if t.currentCall == nil {
		return
	}
	t.callEstablishmentTimer.Stop()
	originator, _ := t.getCallOriginator()
	var replaceWith string
	if from != "" && len(t.currentCall.parties) == 2 {
		// This is a call in progress.
		replaceWith = constCallMsgFinished
	} else {
		// Call hasn't been established. Just drop it.
		replaceWith = constCallMsgDisconnected
	}

	// Send a message indicating the call has ended.
	head := t.currentCall.messageHead()
	msgCopy := *msg
	msgCopy.AsUser = originator.UserId()
	if err := t.saveAndBroadcastMessage(&msgCopy, originator, false, nil, head, replaceWith); err != nil {
		logs.Err.Printf("topic[%s]: failed to write finalizing message for call seq id %d - '%s'", t.name, t.currentCall.seq, err)
	}

	// Send {info} hangup event to the subscribed sessions.
	resp := t.currentCall.infoMessage(constCallEventHangUp)
	t.broadcastToSessions(resp)

	// Let all other sessions know the call is over.
	for tgt := range t.perUser {
		t.infoCallSubsOffline(from, tgt, constCallEventHangUp, t.currentCall.seq, nil, "", true)
	}
	t.currentCall = nil
}

// Server initiated call termination.
func (t *Topic) terminateCallInProgress() {
	if t.currentCall == nil {
		return
	}
	uid, sess := t.getCallOriginator()
	if sess == nil || uid.IsZero() {
		// Just drop the call.
		logs.Warn.Printf("topic[%s]: video call (seq %d) has no originator. Terminating.", t.name, t.currentCall.seq)
		t.currentCall = nil
		return
	}
	// Dummy hangup request.
	dummy := &ClientComMessage{
		Original:  t.original(uid),
		RcptTo:    uid.UserId(),
		AsUser:    uid.UserId(),
		Timestamp: types.TimeNow(),
		sess:      sess,
	}

	t.maybeEndCallInProgress("", dummy)
}
