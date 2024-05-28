package choose_fonts

import (
	"fmt"
	"maps"
	"math"
	"strconv"
	"strings"
	"sync"

	"kitty/tools/tui"
	"kitty/tools/tui/loop"
	"kitty/tools/utils"
	"kitty/tools/utils/shlex"
	"kitty/tools/wcswidth"
)

var _ = fmt.Print

type face_panel struct {
	handler *handler

	family, which       string
	settings            faces_settings
	current_preview     *RenderedSampleTransmit
	current_preview_key faces_preview_key
	preview_cache       map[faces_preview_key]map[string]RenderedSampleTransmit
	preview_cache_mutex sync.Mutex
}

func (self *face_panel) variable_spec(named_style string, axis_overrides map[string]float64) string {
	vname := self.current_preview.Variable_data.Variations_postscript_name_prefix
	ans := fmt.Sprintf(`family=%s variable_name=%s`, shlex.Quote(self.family), shlex.Quote(vname))
	if axis_overrides != nil {
		axis_values := self.current_preview.current_axis_values()
		maps.Copy(axis_values, axis_overrides)
		for tag, val := range axis_values {
			ans += fmt.Sprintf(" %s=%g", tag, val)
		}
	} else if named_style != "" {
		ans += fmt.Sprintf(" style=%s", shlex.Quote(named_style))
	}
	return ans
}

func (self *face_panel) render_lines(start_y int, lines ...string) (y int) {
	sz, _ := self.handler.lp.ScreenSize()
	_, y, str := self.handler.render_lines.InRectangle(lines, 0, start_y, int(sz.WidthCells), int(sz.HeightCells)-y, &self.handler.mouse_state, self.on_click)
	self.handler.lp.QueueWriteString(str)
	return
}

const current_val_style = "fg=cyan bold"
const control_name_style = "fg=yellow bright bold"

func (self *face_panel) draw_axis(sz loop.ScreenSize, y int, ax VariableAxis, axis_value float64) int {
	lp := self.handler.lp
	buf := strings.Builder{}
	buf.WriteString(fmt.Sprintf("%s: ", lp.SprintStyled(control_name_style, utils.IfElse(ax.Strid != "", ax.Strid, ax.Tag))))
	num_of_cells := int(sz.WidthCells) - wcswidth.Stringwidth(buf.String())
	if num_of_cells < 5 {
		return y
	}
	frac := (min(axis_value, ax.Maximum) - ax.Minimum) / (ax.Maximum - ax.Minimum)
	current_cell := int(math.Floor(frac * float64(num_of_cells-1)))
	for i := 0; i < num_of_cells; i++ {
		buf.WriteString(utils.IfElse(i == current_cell, lp.SprintStyled(current_val_style, `⬤`),
			tui.InternalHyperlink("•", fmt.Sprintf("axis:%d/%d:%s", i, num_of_cells-1, ax.Tag))))
	}
	return self.render_lines(y, buf.String())
}

func (self *face_panel) draw_variable_fine_tune(sz loop.ScreenSize, start_y int, preview RenderedSampleTransmit) (y int, err error) {
	s := styles_for_variable_data(preview.Variable_data)
	lines := []string{}
	lp := self.handler.lp
	for _, sg := range s.style_groups {
		if len(sg.styles) < 2 {
			continue
		}
		formatted := make([]string, len(sg.styles))
		for i, style_name := range sg.styles {
			if style_name == preview.Variable_named_style.Name {
				formatted[i] = self.handler.lp.SprintStyled(current_val_style, style_name)
			} else {
				formatted[i] = tui.InternalHyperlink(style_name, "variable_style:"+style_name)
			}
		}
		line := lp.SprintStyled(control_name_style, sg.name) + ": " + strings.Join(formatted, ", ")
		lines = append(lines, line)
	}
	y = self.render_lines(start_y, lines...)
	sub_title := "Fine tune the appearance by clicking in the variable axes below:"
	axis_values := self.current_preview.current_axis_values()
	for _, ax := range self.current_preview.Variable_data.Axes {
		if ax.Hidden {
			continue
		}
		if sub_title != "" {
			y = self.render_lines(y+1, sub_title, "")
			sub_title = ``
		}
		y = self.draw_axis(sz, y, ax, axis_values[ax.Tag])
	}
	return y, nil
}

func (self *face_panel) draw_family_style_select(_ loop.ScreenSize, start_y int, preview RenderedSampleTransmit) (y int, err error) {
	lp := self.handler.lp
	s := styles_in_family(self.family, self.handler.listing.fonts[self.family])
	lines := []string{}
	for _, sg := range s.style_groups {
		formatted := make([]string, len(sg.styles))
		for i, style_name := range sg.styles {
			if style_name == preview.Style {
				formatted[i] = lp.SprintStyled(current_val_style, style_name)
			} else {
				formatted[i] = tui.InternalHyperlink(style_name, "style:"+style_name)
			}
		}
		line := lp.SprintStyled(control_name_style, sg.name) + ": " + strings.Join(formatted, ", ")
		lines = append(lines, line)
	}
	y = self.render_lines(start_y, lines...)
	return y, nil
}

func (self *face_panel) draw_font_features(_ loop.ScreenSize, start_y int, preview RenderedSampleTransmit) (y int, err error) {
	lp := self.handler.lp
	y = start_y
	if len(preview.Features) == 0 {
		return
	}
	formatted := make([]string, 0, len(preview.Features))
	for feat_tag, data := range preview.Features {
		var text string
		if preview.Applied_features[feat_tag] != "" {
			text = preview.Applied_features[feat_tag]
			text = strings.Replace(text, "+", lp.SprintStyled("fg=green", "+"), 1)
			text = strings.Replace(text, "-", lp.SprintStyled("fg=red", "-"), 1)
			text = strings.Replace(text, "=", lp.SprintStyled("fg=cyan", "="), 1)
			if data.Name != "" {
				text = fmt.Sprintf("%s: %s", data.Name, text)
			}
		} else {
			text = utils.IfElse(data.Name == "", feat_tag, data.Name)
			text = lp.SprintStyled("dim", text)
		}
		formatted = append(formatted, tui.InternalHyperlink(text, "feature:"+feat_tag))
	}
	utils.SortWithKey(formatted, func(a string) string {
		return strings.ToLower(wcswidth.StripEscapeCodes(a))
	})
	line := lp.SprintStyled(control_name_style, `Features`) + ": " + strings.Join(formatted, ", ")
	y = self.render_lines(start_y, ``, line)
	return
}

func (self *handler) draw_preview_header(x int) {
	sz, _ := self.lp.ScreenSize()
	width := int(sz.WidthCells) - x
	p := center_string(self.lp.SprintStyled("italic", " preview "), width, "─")
	self.lp.QueueWriteString(self.lp.SprintStyled("dim", p))
}

func (self *face_panel) render_preview(key faces_preview_key) {
	var r map[string]RenderedSampleTransmit
	s := key.settings
	self.handler.set_worker_error(kitty_font_backend.query("render_family_samples", map[string]any{
		"text_style": self.handler.text_style, "font_family": s.font_family,
		"bold_font": s.bold_font, "italic_font": s.italic_font, "bold_italic_font": s.bold_italic_font,
		"width": key.width, "height": key.height, "output_dir": self.handler.temp_dir,
	}, &r))
	self.preview_cache_mutex.Lock()
	defer self.preview_cache_mutex.Unlock()
	self.preview_cache[key] = r
}

func (self *face_panel) draw_screen() (err error) {
	lp := self.handler.lp
	lp.SetCursorVisible(false)
	sz, _ := lp.ScreenSize()
	styled := lp.SprintStyled
	wt := "Regular"
	switch self.which {
	case "bold_font":
		wt = "Bold"
	case "italic_font":
		wt = "Italic"
	case "bold_italic_font":
		wt = "Bold-Italic font"
	}

	lp.QueueWriteString(self.handler.format_title(fmt.Sprintf("%s: %s face", self.family, wt), 0))

	lines := []string{
		fmt.Sprintf("Press %s to accept any changes or %s to cancel. Click on a style name below to switch to it.", styled("fg=green", "Enter"), styled("fg=red", "Esc")), "",
		fmt.Sprintf("Current setting: %s", self.get()), "",
	}
	y := self.render_lines(2, lines...)

	num_lines_per_font := (int(sz.HeightCells) - y - 1) - 2
	num_lines := max(1, num_lines_per_font)
	key := faces_preview_key{settings: self.settings, width: int(sz.WidthCells * sz.CellWidth), height: int(sz.CellHeight) * num_lines}
	self.current_preview_key = key
	self.preview_cache_mutex.Lock()
	defer self.preview_cache_mutex.Unlock()
	previews, found := self.preview_cache[key]
	if !found {
		self.preview_cache[key] = make(map[string]RenderedSampleTransmit)
		go func() {
			self.render_preview(key)
			self.handler.lp.WakeupMainThread()
		}()
		return
	}
	if len(previews) < 4 {
		return
	}
	preview := previews[self.which]
	self.current_preview = &preview
	if len(preview.Variable_data.Axes) > 0 {
		y, err = self.draw_variable_fine_tune(sz, y, preview)
	} else {
		y, err = self.draw_family_style_select(sz, y, preview)
	}
	if err != nil {
		return err
	}
	if y, err = self.draw_font_features(sz, y, preview); err != nil {
		return err
	}

	num_lines = int(math.Ceil(float64(preview.Canvas_height) / float64(sz.CellHeight)))
	if int(sz.HeightCells)-y >= num_lines+2 {
		y++
		lp.MoveCursorTo(1, y+1)
		self.handler.draw_preview_header(0)
		y++
		lp.MoveCursorTo(1, y+1)
		self.handler.graphics_manager.display_image(0, preview.Path, preview.Canvas_width, preview.Canvas_height)
	}
	return
}

func (self *face_panel) initialize(h *handler) (err error) {
	self.handler = h
	self.preview_cache = make(map[faces_preview_key]map[string]RenderedSampleTransmit)
	return
}

func (self *face_panel) on_wakeup() error {
	return self.handler.draw_screen()
}

func (self *face_panel) get() string {
	switch self.which {
	case "font_family":
		return self.settings.font_family
	case "bold_font":
		return self.settings.bold_font
	case "italic_font":
		return self.settings.italic_font
	case "bold_italic_font":
		return self.settings.bold_italic_font
	}
	panic(fmt.Sprintf("Unknown self.which value: %s", self.which))
}

func (self *face_panel) set(setting string) {
	switch self.which {
	case "font_family":
		self.settings.font_family = setting
	case "bold_font":
		self.settings.bold_font = setting
	case "italic_font":
		self.settings.italic_font = setting
	case "bold_italic_font":
		self.settings.bold_italic_font = setting
	}
}

type ParsedFontFeature struct {
	tag     string
	val     uint
	is_bool bool
}

func (self ParsedFontFeature) String() string {
	if self.is_bool {
		return utils.IfElse(self.val == 0, "-", "+") + self.tag
	}
	return fmt.Sprintf("%s=%d", self.tag, self.val)
}

type settable_string struct {
	val    string
	is_set bool
}

type FontSpec struct {
	family, style, postscript_name, full_name, system, variable_name settable_string
	axes                                                             map[string]float64
	features                                                         []ParsedFontFeature
}

func (self FontSpec) String() string {
	if self.system.val != "" {
		return self.system.val
	}
	ans := strings.Builder{}
	a := func(k string, v settable_string) {
		if v.is_set {
			ans.WriteString(fmt.Sprintf(" %s=%s", k, shlex.Quote(v.val)))
		}
	}
	a(`family`, self.family)
	a(`style`, self.style)
	a(`postscript_name`, self.postscript_name)
	a(`full_name`, self.full_name)
	a(`variable_name`, self.variable_name)
	for name, val := range self.axes {
		a(name, settable_string{strconv.FormatFloat(val, 'f', -1, 64), true})
	}
	if len(self.features) > 0 {
		buf := strings.Builder{}
		for _, f := range self.features {
			buf.WriteString(f.String())
			buf.WriteString(" ")
		}
		a(`features`, settable_string{strings.TrimSpace(buf.String()), true})
	}
	return strings.TrimSpace(ans.String())
}

func NewParsedFontFeature(x string) (ans ParsedFontFeature, err error) {
	if x != "" {
		if x[0] == '+' || x[1] == '-' {
			return ParsedFontFeature{x[1:], utils.IfElse(x[0] == '+', uint(1), uint(0)), true}, nil
		} else {
			tag, val, found := strings.Cut(x, "=")
			pff := ParsedFontFeature{tag: tag}
			if found {
				v, err := strconv.ParseUint(val, 10, 0)
				if err != nil {
					return ans, err
				}
				pff.val = uint(v)
			}
			return pff, nil
		}
	}
	return
}

func NewFontSpec(spec string) (ans FontSpec, err error) {
	if spec == "" || spec == "auto" {
		ans.system = settable_string{"auto", true}
		return
	}
	parts, err := shlex.Split(spec)
	if err != nil {
		return
	}
	if !strings.Contains(parts[0], "=") {
		ans.system = settable_string{spec, true}
		return
	}
	for _, item := range parts {
		k, v, found := strings.Cut(item, "=")
		if !found {
			return ans, fmt.Errorf(fmt.Sprintf("The font specification %s is invalid as %s does not contain an =", spec, item))
		}
		switch k {
		case "family":
			ans.family = settable_string{v, true}
		case "style":
			ans.style = settable_string{v, true}
		case "full_name":
			ans.full_name = settable_string{v, true}
		case "postscript_name":
			ans.postscript_name = settable_string{v, true}
		case "variable_name":
			ans.variable_name = settable_string{v, true}
		case "features":
			for _, x := range utils.NewSeparatorScanner(v, " ").Split(v) {
				pff, err := NewParsedFontFeature(x)
				if err != nil {
					return ans, err
				}
				ans.features = append(ans.features, pff)
			}
		default:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return ans, err
			}
			ans.axes[k] = f
		}
	}
	return
}

func (self *face_panel) update_feature_in_setting(value string) error {
	fs, err := NewFontSpec(self.get())
	if err != nil {
		return err
	}
	pff, err := NewParsedFontFeature(value)
	if err != nil {
		return err
	}
	found := false
	for _, f := range fs.features {
		if f.tag == pff.tag {
			f.val = pff.val
			found = true
			break
		}
	}
	if !found {
		fs.features = append(fs.features, pff)
	}
	self.set(fs.String())
	return nil
}

func (self *face_panel) handle_click_on_feature(feat_tag string) error {
	d := self.current_preview.Features[feat_tag]
	if d.Is_index {
	} else {
		for q, serialized := range self.current_preview.Applied_features {
			if q == feat_tag {
				if serialized != "" && (serialized[0] == '+' || strings.HasSuffix(serialized, "=1")) {
					return self.update_feature_in_setting("-" + feat_tag)
				} else {
					return self.update_feature_in_setting("+" + feat_tag)
				}
			}
		}
		return self.update_feature_in_setting("+" + feat_tag)
	}
	return nil
}

func (self *face_panel) on_click(id string) (err error) {
	scheme, val, _ := strings.Cut(id, ":")
	switch scheme {
	case "style":
		self.set(fmt.Sprintf(`family="%s" style="%s"`, self.family, val))
	case "variable_style":
		self.set(self.variable_spec(val, nil))
	case "feature":
		if err = self.handle_click_on_feature(val); err != nil {
			return err
		}
	case "axis":
		p, tag, _ := strings.Cut(val, ":")
		num, den, _ := strings.Cut(p, "/")
		n, _ := strconv.Atoi(num)
		d, _ := strconv.Atoi(den)
		frac := float64(n) / float64(d)
		for _, ax := range self.current_preview.Variable_data.Axes {
			if ax.Tag == tag {
				axval := ax.Minimum + (ax.Maximum-ax.Minimum)*frac
				self.set(self.variable_spec("", map[string]float64{tag: axval}))
				break
			}
		}
	}
	// Render preview synchronously to void flashing
	key := self.current_preview_key
	key.settings = self.settings
	self.preview_cache_mutex.Lock()
	previews := self.preview_cache[key]
	self.preview_cache_mutex.Unlock()
	if len(previews) < 4 {
		self.render_preview(key)
	}
	return self.handler.draw_screen()
}

func (self *face_panel) on_key_event(event *loop.KeyEvent) (err error) {
	if event.MatchesPressOrRepeat("esc") {
		event.Handled = true
		self.handler.current_pane = &self.handler.faces
		return self.handler.draw_screen()
	} else if event.MatchesPressOrRepeat("enter") {
		event.Handled = true
		self.handler.current_pane = &self.handler.faces
		self.handler.faces.settings = self.settings
		return self.handler.draw_screen()
	}
	return
}

func (self *face_panel) on_text(text string, from_key_event bool, in_bracketed_paste bool) (err error) {
	return
}

func (self *face_panel) on_enter(family, which string, settings faces_settings) error {
	self.family = family
	self.settings = settings
	self.which = which
	self.handler.current_pane = self
	return self.handler.draw_screen()
}
