<script lang="ts">
  let { value = $bindable(''), onsubmit }: { value: string; onsubmit: (text: string) => void } = $props();

  function submit(event: SubmitEvent) {
    event.preventDefault();
    const text = value.trim();
    if (!text) return;
    onsubmit(text);
    value = '';
  }
</script>

<form class="composer" onsubmit={submit}>
  <textarea
    data-testid="composer-input"
    bind:value
    rows="2"
    placeholder="Tanya sesuatu…"
    aria-label="Pesan"
  ></textarea>
  <button data-testid="send-button" type="submit">Kirim</button>
</form>

<style>
  .composer {
    display: flex;
    align-items: flex-end;
    gap: 10px;
    padding: 12px;
    border: 1px solid var(--line);
    border-radius: 16px;
    background: var(--surface);
  }
  textarea {
    flex: 1;
    resize: none;
    border: 0;
    background: transparent;
    color: var(--text);
    font: inherit;
    line-height: 1.6;
    outline: none;
  }
  button {
    padding: 8px 16px;
    border-radius: 6px;
    background: var(--text);
    color: var(--ground);
    font-weight: 600;
  }
  button:hover { background: var(--accent); }
</style>
