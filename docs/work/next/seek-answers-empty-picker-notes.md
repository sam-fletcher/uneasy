---
title: "Seek Answers: say an empty picker's reason once in its other two sub-steps"
order: 78
---

A break-a-resource pick with nothing to break used to print "No resource can be broken" twice (the empty
picker, then the forfeit note); the note now carries only "This pick has no valid target." The other two
sub-steps have the same picker-plus-note shape, in different words:

- Learn an asset's secrets: "No assets available." then "No asset's secrets remain to learn — this pick has no
  valid target."
- The mar penalty: "No eligible resources of your own remain." then "None of your resources can be broken —
  this penalty can't be applied."

Decide each pair's wording together, since the picker's empty message and the note should read as one
explanation (as the break-a-resource pair now does), then change both and pin them in
`SeekAnswersPanel.test.ts`.

Also in this panel: the stepper's note reads "Learn the secrets of an asset— no valid targets", because the
note's leading space is trimmed inside its span.
