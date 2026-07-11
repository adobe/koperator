// Collapsible + searchable tree behavior for CRD reference pages
// (layout: crd). The generated markup is a flat sequence of sibling
// <div class="property depth-N"> blocks - there is no real DOM nesting -
// so "descendants" of a property are simply the following siblings whose
// depth is greater, up to the next sibling at the same or lower depth.
(function () {
  function init() {
    var properties = Array.prototype.slice.call(document.querySelectorAll('.property'));
    if (!properties.length) return;

    function depthOf(el) {
      var m = el.className.match(/\bdepth-(\d+)\b/);
      return m ? parseInt(m[1], 10) : 0;
    }

    properties.forEach(function (el) {
      el._depth = depthOf(el);
    });

    function hasChildren(i) {
      var next = properties[i + 1];
      return !!next && next._depth > properties[i]._depth;
    }

    function setCollapsed(i, collapsed) {
      var el = properties[i];
      var header = el.querySelector('.property-header');
      el.classList.toggle('collapsed', collapsed);
      if (header) header.setAttribute('aria-expanded', String(!collapsed));
      var depth = el._depth;
      for (var j = i + 1; j < properties.length; j++) {
        if (properties[j]._depth <= depth) break;
        properties[j].classList.toggle('js-hidden', collapsed);
      }
    }

    properties.forEach(function (el, i) {
      if (!hasChildren(i)) return;
      var header = el.querySelector('.property-header');
      if (!header) return;
      header.classList.add('has-children');
      header.setAttribute('role', 'button');
      header.setAttribute('tabindex', '0');
      header.setAttribute('aria-expanded', 'true');

      function toggle() {
        setCollapsed(i, !el.classList.contains('collapsed'));
      }
      header.addEventListener('click', function (e) {
        // Docsy injects a hidden-until-hover "#" permalink link
        // (.td-heading-self-link) into every heading, including these. On
        // hover it appears right where the cursor already is, so a header
        // click often lands on it too - that navigates to the in-page
        // anchor (shifting scroll/hash) without necessarily undoing our
        // toggle, making a single click look like it "did nothing". Let the
        // permalink behave normally when clicked directly; otherwise stop it
        // from interfering with the collapse toggle.
        if (e.target.closest('.td-heading-self-link')) return;
        e.preventDefault();
        toggle();
      });
      header.addEventListener('keydown', function (e) {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          toggle();
        }
      });
    });

    // Toolbar: search box + expand/collapse all, inserted right before the
    // first property block.
    var toolbar = document.createElement('div');
    toolbar.className = 'crd-toolbar';
    toolbar.innerHTML =
      '<input type="search" class="crd-search" placeholder="Filter properties by name or description…" aria-label="Filter properties">' +
      '<button type="button" class="crd-expand-all">Expand all</button>' +
      '<button type="button" class="crd-collapse-all">Collapse all</button>' +
      '<span class="crd-search-count"></span>';
    properties[0].parentNode.insertBefore(toolbar, properties[0]);

    var searchInput = toolbar.querySelector('.crd-search');
    var countEl = toolbar.querySelector('.crd-search-count');

    function clearSearchState() {
      properties.forEach(function (el) {
        el.classList.remove('js-hidden');
      });
      countEl.textContent = '';
    }

    toolbar.querySelector('.crd-expand-all').addEventListener('click', function () {
      searchInput.value = '';
      clearSearchState();
      properties.forEach(function (el, i) {
        if (hasChildren(i)) setCollapsed(i, false);
      });
    });

    toolbar.querySelector('.crd-collapse-all').addEventListener('click', function () {
      searchInput.value = '';
      clearSearchState();
      // Collapse deepest-first so a parent's collapse doesn't get
      // re-expanded by processing order.
      for (var i = properties.length - 1; i >= 0; i--) {
        if (hasChildren(i)) setCollapsed(i, true);
      }
    });

    searchInput.addEventListener('input', function () {
      var term = searchInput.value.trim().toLowerCase();

      properties.forEach(function (el) {
        el.classList.remove('collapsed');
      });

      if (!term) {
        clearSearchState();
        return;
      }

      var visible = new Array(properties.length).fill(false);
      var matchCount = 0;

      properties.forEach(function (el, i) {
        var header = el.querySelector('.property-header');
        var desc = el.querySelector('.property-description');
        var haystack = (header ? header.textContent : '') + ' ' + (desc ? desc.textContent : '');
        if (haystack.toLowerCase().indexOf(term) === -1) return;

        visible[i] = true;
        matchCount++;

        // Reveal ancestors: walk backward through strictly decreasing depth.
        var d = el._depth;
        for (var j = i - 1; j >= 0 && d > 0; j--) {
          if (properties[j]._depth < d) {
            visible[j] = true;
            d = properties[j]._depth;
          }
        }
      });

      properties.forEach(function (el, i) {
        el.classList.toggle('js-hidden', !visible[i]);
      });

      countEl.textContent = matchCount + (matchCount === 1 ? ' match' : ' matches');
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
