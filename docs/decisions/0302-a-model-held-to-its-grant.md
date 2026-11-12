# 0302. A model held to its grant

- Status: Accepted (porting's B103, 2026-11-04). Extended the same night to every Arista model's grant and to channels a grant leaves out, from porting's reading of the FCC grants
- Date: 2026-11-04
- Proposed by: the OpenWrt porting session, from the Arista APs' FCC grants. Written up by Claude
- Refines: 0089 (what a radio can do, from its htmodes)

## Context

- **Aeolus takes what a radio can do from the htmodes it reports** (0089). The config check and the hardware a folder offers both read `radio.Can`.
- **A C-360's 5 GHz radio (QCN9074) reports VHT160 and HE160.** Its self-managed regulatory rules allow 5490–5730 MHz at 160 MHz.
- **The C-360's FCC grant doesn't cover 160 MHz on 5 GHz** (TOR-C360, 2022-03-02; the C2PC of 2022-05-17 added U-NII-2A and 2C). 5 GHz is granted at 20, 40 and 80 MHz only. Its 6 GHz grant (15E, low-power indoor) states 20, 40, 80 and 160 MHz.
- **Porting read every Arista model's grant from fccid.io** (Arista APs `build/compliance/fcc-grants.md`, grantee TOR):
  - Every model: 2.4 GHz 20/40, 5 GHz 20/40/80, no contiguous 160 on 5 GHz. The C-230, C-130 and C-250 list 80+80.
  - The C-360 and W-318: 6 GHz LPI 20–160.
  - The W-118 and C-250: U-NII-2C is 5500–5700 MHz, so channel 144 (5720) is outside the grant, at any width.
  - Every model: U-NII-4 (169–177) is uncovered. The radios' own board data already stops at 165, and the W-318's is held by port 0077.
  - The C-260 has no grant under TOR. Its FCC ID is on its label, not yet read.
- **Porting also compared each C-360 radio's reported power on -112 with the grant's maximum conducted output.** All are at or under it, for example 57ff at 23 dBm on 124 against 23.0, and C360-AP at 25 on 149 against 28.4. The other models aren't measured yet.
- **Today every Aeolus AP runs 5 GHz at 40 MHz,** so nothing is out of its grant now. But Aeolus would have taken 160 had anyone set it.

## Decision

- **`radio.Granted`, each model's grant where its radios would do more,** keyed by the board name the AP reports (`facts.board`): the widest on each band (`Most`), and the channels of each band the grant leaves out (`Off`). Each row says which grant it is from.
  - `arista,c-360`, `arista,w-318`: 2.4 GHz 40, 5 GHz 80, 6 GHz 160.
  - `arista,c-200`, `arista,c-230`, `arista,c-130`: 2.4 GHz 40, 5 GHz 80.
  - `arista,w-118`, `arista,c-250`: 2.4 GHz 40, 5 GHz 80, and not 5 GHz channel 144.
  - `arista,c-260`: 5 GHz 80, as every grant read is, until its label is read.
  - The C-130 and C-250 aren't ported. Their board names follow the port's naming of the others.
- **`radio.CanOn(board, band, htmodes)` is what the radio reports, no wider than the grant.** The config check (compose) and the hardware view read it instead of `radio.Can`. A width past the grant is a config problem of its own: "this AP's model is certified for at most 80 MHz on this band".
- **A channel the grant leaves out is refused by the config check:**
  - set by hand, or one whose block at the width uses it (a W-118 on 140 at 40 MHz, or anywhere in 132–144 at 80);
  - named among the channels an automatic channel may be;
  - and an automatic channel from any channel at all, which could be it, unless DFS avoided keeps the radio off every such channel. A set without it is what's asked for.
- **A model or band not in the table is held only to what its radio reports.** An OpenWrt One's 5 GHz still offers 160.
- **U-NII-4 isn't a row:** no Arista radio's board data offers 169–177, and channel compatibility (0232), on unless turned off, keeps every radio off 165–177 as well.

## Consequences

- **No Arista AP can be given 160 MHz on 5 GHz,** whether on itself or from a folder above it. The C-360's and W-318's 6 GHz can still run 160.
- **A folder with any Arista AP under it offers at most 80 MHz on 5 GHz,** as a band setting applies to every radio of that band.
- **A W-118 or C-250, if one is ever enrolled, needs a 5 GHz channel set without 144, or DFS avoided,** before its config is taken.
- **Nothing on any AP changes:** none runs past its grant today, and no W-118 is an Aeolus AP.
- **More rows are added as more grants are read.** The C-260's label is owed.
