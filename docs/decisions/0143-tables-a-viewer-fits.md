# 0143. Tables a viewer fits to their own eye

- Status: Proposed
- Date: 2026-10-12
- Proposed by: Griff, looking at a table whose rows ran tall: "making all the rows slideable and being able to hide or show columns might help", and, asked what slideable was, "expand or contract them at will, some are unnecessarily long or tall and can be fixed with expanding the column". Written up by Claude
- Refines: 0042

## Context

- **Every list in the UI is a table laid out by the browser,** each column as wide as its share of the page. A column with much in it, an AP's radios or a line of networks, wraps, and its row runs tall while the columns beside it stand half empty.
- **What is worth a column differs with who looks and why.** One who reads signal has no use for a maker's name, and the other way about.
- **The UI has no build step and no library** (0042): what it does, it does itself.

## Decision

- **Every list table can be fitted by whoever looks at it.**
  - **A column is made wider or narrower by dragging the right edge of its heading.** From the first drag every column is held at its width, so one made wider makes the table wider and no other narrower; a table wider than its place scrolls sideways within it. A column is never narrower than 48 px.
  - **A column is hidden or shown from a small menu on the heading row,** a slider a column. The last column showing stays.
  - **"As it came" puts the table back:** every column shown, and laid out by the browser again.
- **What was chosen is kept in the browser,** by the table's headings: a table with the same headings on another page is the same table to it. Nothing of it reaches the manager, and another browser, or another person, starts with every table as it came.
- **It is one module, `tables.js`,** that fits each table as it is drawn, the pages' own redraw every half minute too. No page's code knows of it.

## Consequences

- **A row that ran tall is made short** by giving its column room; and a table of many columns is cut to those wanted.
- **A table fitted is as its viewer left it** on every page that has one of those headings, until put back.
- **A row that spans columns,** as one opened under another, is left whole when a column is hidden.
- **It is the browser's alone:** cleared with the browser's own data, and not there in a private window.
- **A heading renamed is a new table** to what was kept; the old choice is left unused.

## Not decided

- **Rows put in an order by dragging,** or sorted by any column: the Clients tab sorts by its own headings, and no other does.
- **A choice kept with the account,** so that it follows a person from one browser to another.
- **Widths by the keyboard.**
