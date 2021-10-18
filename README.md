# In-Memory Expirable Key Counter

This is a fast metrics server, ideal for tracking throttling. Put values to the server, and then count them.
Values expire according to the TTL seconds you set.

This repo provides both the client and server. 

## Why

Redis would be the natural choice for this type of service, but it lacks a native "count" operation for keys.
