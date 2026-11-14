# Subscribers are account-less

A Subscriber has no username or password; they exist only through verified Contact Channels (email, phone, push device), and they manage Watched Addresses and unsubscribe through signed, expiring magic links sent to a verified channel. We chose this over conventional accounts because the data we hold (home address + contact details) is sensitive, and accounts would add credentials to protect, password resets and a login surface without giving residents anything they need. "Unsubscribe any time" and GDPR deletion both fall out naturally: removing the last Contact Channel deletes the Subscriber.

## Consequences

- The mobile app attaches a push device to an existing Subscriber by confirming a magic link, not by logging in.
- Anyone who controls a verified Contact Channel controls the Subscriber's Watched Addresses; magic links must be short-lived and single-use.
