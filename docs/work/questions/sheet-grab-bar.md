---
title: "Should a sheet show a grab bar now that it can be swiped away?"
checked: "DESIGN_PRINCIPLES and DECISIONS (grepped for grab, handle, swipe, drag: nothing about sheets) and STYLE_GUIDE's sheet entries (surface, shadow, top-corner radius, title size): none mentions a handle, so it is open."
---

A sheet now closes when it is dragged down on a phone, but nothing on it says so:
the only visible controls are the × and the scrim. Many phone apps show a small
rounded bar centred at the top of a sheet to say "this moves". I did not add one,
because it changes the screen anatomy (a new mark on every sheet, and the title
row would have to make room for it). If the swipe turns out to go undiscovered,
a bar is the usual answer; if the owner is happy finding it by habit, leave it.
