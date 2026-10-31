// The "Copy" button of a message to hand over (an invitation or recovery link): copies the read-only box next to it, and
// selects the text when the clipboard is unavailable (plain http on a LAN).
document.addEventListener('alpine:init', () => {
  Alpine.data('copyLink', () => ({
    copied: false,
    failed: false,
    copy() {
      const box = this.$refs.msg;
      const fail = () => { box.select(); this.failed = true; };
      const done = () => { this.failed = false; this.copied = true; setTimeout(() => { this.copied = false; }, 2000); };
      if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(box.value).then(done).catch(fail);
        return;
      }
      box.select();
      try { if (document.execCommand('copy')) done(); else this.failed = true; } catch (e) { this.failed = true; /* the text stays selected */ }
    },
  }));
});
