// The "Copy" button of a message to hand over (an invitation or recovery link): copies the read-only box next to it, and
// selects the text when the clipboard is unavailable (plain http on a LAN).
document.addEventListener('alpine:init', () => {
  Alpine.data('copyLink', () => ({
    copied: false,
    copy() {
      const box = this.$refs.msg;
      const done = () => { this.copied = true; setTimeout(() => { this.copied = false; }, 2000); };
      if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(box.value).then(done).catch(() => box.select());
        return;
      }
      box.select();
      try { if (document.execCommand('copy')) done(); } catch (e) { /* the text stays selected */ }
    },
  }));
});
