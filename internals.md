# Internals
This document will describe the architecture of the project.
## Overview
This project uses redis streams. A conversation has the following data associated with it:
- A stream of chat events stored in the key \<conversationID>chatstream. These chat events are of the form { sender : *string*, data: *protobuf encoded data* }.
- A set of user strings stored in the key \<conversationID>users
- Metadata in the hash <conversationID>metadata
For this first iteration users are just strings, there's no security or UUID (so any two users with the same name are the same user). Here's the data associated with a user in redis.
- A set of convsersation IDs stored in the key \<user>conversations
- A pub/sub key called \<user>notifyConvs for notifying users when their conversation membership changes

## Next Steps
- Have a persistent storage consume from the redis stream. Put a size limit on conversations stored in redis and satisfy some queries from persistent storage.  
- Use redis cluster.
- Make \<user>conversations ordered by last message and maybe also include all the necessary header data (last message)