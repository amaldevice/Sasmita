/**
 * MyGo trusts every custom scheme loaded in the main frame, so artifact://
 * must never become a top-level URL. Go's RejectMainFrameNavigation is the
 * authoritative guard (OnWillNavigate); this is the frontend belt-and-braces
 * version for links and scripted navigation in the trusted surface.
 */
export function isArtifactURL(url: string): boolean {
  return /^artifact:\/\//i.test(url.trim());
}

export function rejectArtifactNavigation(url: string): boolean {
  return isArtifactURL(url);
}

export function installArtifactNavigationGuard(): () => void {
  const onClick = (event: MouseEvent): void => {
    const target = event.target;
    if (!(target instanceof Element)) return;
    const anchor = target.closest('a[href]');
    if (!anchor) return;
    if (isArtifactURL(anchor.getAttribute('href') ?? '')) {
      event.preventDefault();
    }
  };
  document.addEventListener('click', onClick, true);
  return () => document.removeEventListener('click', onClick, true);
}
