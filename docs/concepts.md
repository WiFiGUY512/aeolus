# How Aeolus holds a network

## Two trees

Everything is set in two trees under the **Org**:

- **Locations: where the APs are.** Folders such as a site, a building or a floor, with the APs in them. A folder sets radio settings (bands, channels, power) and system settings (time zone, updates), and says which service folders apply.
- **Services: what the APs offer.** Networks (an SSID with its security, VLAN or tunnel, and roaming), in folders of their own.

An AP serves the networks its location's folders take from the services tree.

## Settings inherit, field by field

A setting made on a folder holds for everything below it, field by field. A folder or AP below can set a field differently, an override, and keep every other field as inherited. Each value shows where it comes from: set here, inherited from a folder above, locked, or Aeolus's default.

- **Lock** a field on a folder, and nothing below may set it otherwise.
- **Break Hierarchy** on a folder starts a branch of its own: from there down, nothing is inherited from above it.

## Every change is logged

Each change is one entry in the change log, with who made it and when, and a note where it helps. Before a change that reaches APs, **Preview** shows which APs it touches and what each will run.

## What happens on an AP

1. The manager gives each AP its config: what its folders resolve to.
2. The agent renders that as OpenWrt config and has the manager check the result.
3. The agent applies it, with an automatic revert if the AP loses the manager.
4. It reports what it runs.

If the manager is away, the AP keeps running what it has. New APs wait in the **Landing Zone** until someone adopts them into a folder.

## Releases

The manager runs one channel, **stable** or **testing**. The APs take their agent from the manager. See [install.md](install.md#updates).
