/**
 * Whether a listener is publishing to a room, as shown in the lobby and on the
 * organiser's page.
 *
 * Its own component because the two were written separately and had drifted:
 * an idle room was amber in one place and red in the other, which reads as an
 * error rather than as a room between talks, and only one of them hid the
 * decorative dot from a screen reader.
 */
export default function RoomBadge({ live }: { live: boolean }) {
  return (
    <span className={`badge badge-${live ? 'live' : 'waiting'}`}>
      <span className="badge-dot" aria-hidden="true" />
      {live ? 'Live' : 'Idle'}
    </span>
  )
}
