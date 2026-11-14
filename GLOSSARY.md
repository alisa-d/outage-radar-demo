# Outage Radar

Shows residents planned and emergency water and power outages for their area, collected from many utility providers in one place, and alerts subscribers whose addresses are affected.

## Geography

**Region**:
The area a user picks on the main page, one per regional centre (e.g. Varna region). Contains many Settlements.
_Avoid_: City (when meaning the selectable area)

**Settlement**:
A single town or village. Every Address belongs to exactly one Settlement.
_Avoid_: City, town, village (as model terms)

**Service Territory**:
The set of Settlements a Provider serves for one Utility.

**Address**:
A canonical location from the address registry: a street and building number in a Settlement, with coordinates. Never free text.
_Avoid_: Location, place

## Providers and outages

**Utility**:
The kind of service that can be interrupted: water or power.

**Provider**:
An organisation that delivers a Utility in a Service Territory and publishes Notices (e.g. ВиК Варна, ЕРП Север).
_Avoid_: Supplier, source, company

**Notice**:
A provider's published announcement, kept verbatim as the source of truth. One Notice yields one or more Outages.
_Avoid_: Announcement, post, message

**Outage**:
One interruption of a Utility: a time window, an Affected Area and a kind (planned or emergency), extracted from a Notice.
_Avoid_: Interruption, incident, event

**Affected Area**:
The geographic extent of an Outage as resolved from the Notice's description (streets, enclosed blocks, landmarks, whole settlements), carrying a confidence level.

**Severity**:
How bad an Outage is: a **Cut** (no supply) or a **Disturbance** (possible interruptions). Independent of the Match Result.

**Unlocated**:
An Outage in a city whose Affected Area could not be resolved at all. It is shown publicly but triggers no Alerts until an Operator places it.

**Gazetteer**:
The named areas and landmarks of a Settlement (quarters, местности, resorts, landmarks) with their geometry, used to resolve Affected Areas.

**Provider Health**:
Whether we can currently trust a Provider's data: **Healthy**, **Stale** (no successful fetch recently) or **Suspicious** (fetches succeed but the results look wrong).

**Shadow Mode**:
A Provider state in which its Outages are published but the Alerts they would trigger are only recorded, not sent, until an Operator switches the Provider live.

**Coverage**:
What a Provider's published data can tell us about, e.g. "planned power outages only". Absence of an Outage outside Coverage means nothing.

## Matching

**Match Result**:
The answer to "does this Outage affect this Address?": **Affected**, **Possibly affected**, or **Not affected**. **No data** applies when a Provider's data could not be obtained.

## Subscribers and alerts

**Subscriber**:
A person known only through their verified Contact Channels; there are no accounts or passwords.
_Avoid_: User account, customer

**Contact Channel**:
A verified way to reach a Subscriber: an email address, phone number (SMS/Viber) or push device.

**Watched Address**:
An Address a Subscriber asked to be alerted about. A Subscriber has a small number of them.
_Avoid_: Subscription address, saved address

**Alert**:
A message telling a Subscriber that an Outage newly affects, or has materially changed for, one of their Watched Addresses.
_Avoid_: Notification (reserved for the delivery mechanism)

**Operator**:
Backoffice staff who review Unlocated Outages, curate the Gazetteer and monitor Provider Health. They have no routine access to Subscriber data.
_Avoid_: Admin, moderator

**Quiet Hours**:
The night window during which non-urgent Alerts are held until morning.
